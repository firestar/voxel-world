package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"central/internal/config"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type kubernetesRuntime struct {
	clientset *kubernetes.Clientset
	namespace string
}

func newKubernetesRuntime() (*kubernetesRuntime, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home, herr := os.UserHomeDir(); herr == nil {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
		if kubeconfig == "" {
			return nil, fmt.Errorf("cannot determine kubeconfig: %w", err)
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialise kubernetes client: %w", err)
	}

	namespace, err := detectCurrentNamespace()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	return &kubernetesRuntime{clientset: clientset, namespace: namespace}, nil
}

func (r *kubernetesRuntime) start(ctx context.Context, cfg *config.Config, cs config.ChunkServer) (*process, error) {
	if cs.ContainerImage == "" {
		return nil, fmt.Errorf("container_image must be set for kubernetes runtime (chunk server %s)", cs.ID)
	}

	envMap, err := chunkServerEnvironment(cfg, cs)
	if err != nil {
		return nil, err
	}
	env := make([]corev1.EnvVar, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: cs.ID,
			Labels: map[string]string{
				"app":             "chunk-server",
				"chunk-server-id": cs.ID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "chunk-server",
					Image: cs.ContainerImage,
					Args:  cs.Args,
					Env:   env,
				},
			},
		},
	}

	podClient := r.clientset.CoreV1().Pods(r.namespace)
	if _, err := podClient.Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create kubernetes pod: %w", err)
		}
		if err := r.replacePod(ctx, podClient, pod); err != nil {
			return nil, err
		}
	}

	proc := newProcess(cs)
	proc.setActiveStatus("pending")

	watchCtx, cancel := context.WithCancel(context.Background())
	proc.cancelWatch = cancel

	go r.monitorPod(watchCtx, proc, pod.Name)

	proc.stopFn = func(stopCtx context.Context) error {
		grace := int64(10)
		err := podClient.Delete(stopCtx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	return proc, nil
}

func (r *kubernetesRuntime) monitorPod(ctx context.Context, proc *process, podName string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pod, err := r.clientset.CoreV1().Pods(r.namespace).Get(context.Background(), podName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				proc.setFinalStatus("stopped", fmt.Errorf("pod %s deleted", podName))
				return
			}
			if err != nil {
				proc.setFinalStatus("stopped", err)
				return
			}

			switch pod.Status.Phase {
			case corev1.PodPending:
				proc.setActiveStatus("pending")
			case corev1.PodRunning:
				proc.setActiveStatus("running")
			case corev1.PodSucceeded:
				proc.setFinalStatus("exited", nil)
				return
			case corev1.PodFailed:
				proc.setFinalStatus("stopped", extractPodFailure(pod))
				return
			}
		}
	}
}

func (r *kubernetesRuntime) replacePod(ctx context.Context, podClient typedcorev1.PodInterface, pod *corev1.Pod) error {
	if err := podClient.Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete existing pod: %w", err)
	}

	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := wait.PollUntilContextTimeout(pollCtx, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := podClient.Get(ctx, pod.Name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("wait for pod deletion: %w", err)
	}

	if _, err := podClient.Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("recreate pod: %w", err)
	}
	return nil
}

func (r *kubernetesRuntime) shutdown() {}

func detectCurrentNamespace() (string, error) {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns, nil
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func extractPodFailure(pod *corev1.Pod) error {
	if pod.Status.Message != "" {
		return errors.New(pod.Status.Message)
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil {
			term := status.State.Terminated
			if term.Message != "" {
				return errors.New(term.Message)
			}
			if term.Reason != "" {
				return fmt.Errorf("container terminated: %s (exit code %d)", term.Reason, term.ExitCode)
			}
			return fmt.Errorf("container exited with code %d", term.ExitCode)
		}
	}
	if pod.Status.Reason != "" {
		return fmt.Errorf("pod failed: %s", pod.Status.Reason)
	}
	return errors.New("pod failed")
}
