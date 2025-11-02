import * as THREE from 'three';
import type { ChunkDeltaEvent, ChunkSummaryEvent, WorldTimeState } from '@shared/protocol';

interface ChunkMesh {
  mesh: THREE.Mesh<THREE.BoxGeometry, THREE.MeshStandardMaterial>;
  highlightTimeout?: ReturnType<typeof setTimeout>;
}

export class GameScene {
  private scene: THREE.Scene;
  private camera: THREE.PerspectiveCamera;
  private renderer: THREE.WebGLRenderer;
  private chunks = new Map<string, ChunkMesh>();
  private animationHandle?: number;
  private ambient: THREE.AmbientLight;
  private directional: THREE.DirectionalLight;
  private sunTarget: THREE.Object3D;
  private timeState: WorldTimeState | null = null;
  private keyState = new Set<string>();
  private cameraYaw = Math.PI;
  private readonly cameraHeight = 1000;
  private readonly cameraPitch = -Math.PI / 4;
  private readonly moveSpeed = 400;
  private readonly rotationSpeed = Math.PI / 2;
  private lastFrameTime = performance.now();
  private readonly skyNight = new THREE.Color('#0f172a');
  private readonly skyDay = new THREE.Color('#87ceeb');
  private readonly skyColor = new THREE.Color('#0f172a');

  constructor(private canvas: HTMLCanvasElement) {
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color('#0f172a');

    const aspect = canvas.clientWidth / Math.max(canvas.clientHeight, 1);
    this.camera = new THREE.PerspectiveCamera(60, aspect, 0.1, 10_000);
    this.camera.position.set(0, this.cameraHeight, this.cameraHeight);

    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
    this.renderer.setPixelRatio(window.devicePixelRatio);
    this.renderer.setSize(canvas.clientWidth, canvas.clientHeight);

    this.ambient = new THREE.AmbientLight(0xffffff, 0.6);
    this.directional = new THREE.DirectionalLight(0xffffff, 0.8);
    this.directional.position.set(0, 1200, 0);
    this.directional.castShadow = false;
    this.sunTarget = new THREE.Object3D();
    this.sunTarget.position.set(0, 0, 0);
    this.directional.target = this.sunTarget;

    this.scene.add(this.ambient);
    this.scene.add(this.directional);
    this.scene.add(this.directional.target);

    const grid = new THREE.GridHelper(2000, 40, '#22d3ee', '#1e293b');
    this.scene.add(grid);

    window.addEventListener('resize', () => this.handleResize());
    window.addEventListener('keydown', this.handleKeyDown);
    window.addEventListener('keyup', this.handleKeyUp);
    this.updateCameraOrientation();
    this.start();
  }

  dispose() {
    if (this.animationHandle) {
      cancelAnimationFrame(this.animationHandle);
      this.animationHandle = undefined;
    }
    this.renderer.dispose();
    this.chunks.forEach(({ mesh, highlightTimeout }) => {
      if (highlightTimeout) {
        clearTimeout(highlightTimeout);
      }
      mesh.geometry.dispose();
      mesh.material.dispose();
    });
    this.chunks.clear();
    window.removeEventListener('keydown', this.handleKeyDown);
    window.removeEventListener('keyup', this.handleKeyUp);
  }

  addChunkSummary(event: ChunkSummaryEvent) {
    const key = this.chunkKey(event.serverId, event.summary.chunkX, event.summary.chunkY);
    const existing = this.chunks.get(key);
    const height = Math.min(800, Math.max(50, event.summary.blockCount / 50));
    if (existing) {
      existing.mesh.scale.y = height / 100;
      existing.mesh.position.y = height / 2;
      existing.mesh.material.color.set(this.colorForServer(event.serverId));
      return;
    }

    const geometry = new THREE.BoxGeometry(100, height, 100);
    const material = new THREE.MeshStandardMaterial({
      color: this.colorForServer(event.serverId),
      opacity: 0.9,
      transparent: true
    });
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.set(event.summary.chunkX * 120, height / 2, event.summary.chunkY * 120);
    mesh.castShadow = false;
    mesh.receiveShadow = true;
    this.scene.add(mesh);
    this.chunks.set(key, { mesh });
  }

  applyChunkDelta(event: ChunkDeltaEvent) {
    const key = this.chunkKey(event.serverId, event.delta.chunkX, event.delta.chunkY);
    const entry = this.chunks.get(key);
    if (!entry) {
      // create placeholder to visualise delta even without summary
      const summaryEvent: ChunkSummaryEvent = {
        serverId: event.serverId,
        summary: {
          chunkX: event.delta.chunkX,
          chunkY: event.delta.chunkY,
          version: 0,
          blockCount: 0
        }
      };
      this.addChunkSummary(summaryEvent);
      return this.applyChunkDelta(event);
    }
    entry.mesh.material.emissive = new THREE.Color('#f59e0b');
    if (entry.highlightTimeout) {
      clearTimeout(entry.highlightTimeout);
    }
    entry.highlightTimeout = setTimeout(() => {
      entry.mesh.material.emissive = new THREE.Color('#000000');
    }, 400);
  }

  updateTime(state: WorldTimeState) {
    this.timeState = state;
  }

  private chunkKey(serverId: string, chunkX: number, chunkY: number): string {
    return `${serverId}:${chunkX}:${chunkY}`;
  }

  private colorForServer(serverId: string): string {
    let hash = 0;
    for (let i = 0; i < serverId.length; i += 1) {
      hash = (hash * 31 + serverId.charCodeAt(i)) >>> 0;
    }
    const hue = hash % 360;
    return new THREE.Color(`hsl(${hue}, 70%, 55%)`).getStyle();
  }

  private start() {
    const renderLoop = () => {
      this.animationHandle = requestAnimationFrame(renderLoop);
      const now = performance.now();
      const delta = Math.min((now - this.lastFrameTime) / 1000, 0.1);
      this.lastFrameTime = now;
      this.updateCamera(delta);
      this.updateLighting();
      this.renderer.render(this.scene, this.camera);
    };
    this.lastFrameTime = performance.now();
    renderLoop();
  }

  private handleResize() {
    const width = this.canvas.clientWidth;
    const height = Math.max(this.canvas.clientHeight, 1);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
  }

  private updateCamera(delta: number) {
    let rotated = false;
    if (this.keyState.has('KeyQ')) {
      this.cameraYaw += this.rotationSpeed * delta;
      rotated = true;
    }
    if (this.keyState.has('KeyE')) {
      this.cameraYaw -= this.rotationSpeed * delta;
      rotated = true;
    }
    const forward = new THREE.Vector3(Math.sin(this.cameraYaw), 0, Math.cos(this.cameraYaw));
    const movement = new THREE.Vector3();
    if (this.keyState.has('KeyW')) {
      movement.add(forward);
    }
    if (this.keyState.has('KeyS')) {
      movement.sub(forward);
    }
    const right = new THREE.Vector3(-forward.z, 0, forward.x);
    if (this.keyState.has('KeyD')) {
      movement.add(right);
    }
    if (this.keyState.has('KeyA')) {
      movement.sub(right);
    }
    if (movement.lengthSq() > 0) {
      movement.normalize().multiplyScalar(this.moveSpeed * delta);
      this.camera.position.add(movement);
    }
    this.camera.position.y = this.cameraHeight;
    if (movement.lengthSq() > 0 || rotated) {
      this.updateCameraOrientation();
    }
  }

  private updateCameraOrientation() {
    const cosPitch = Math.cos(this.cameraPitch);
    const forward = new THREE.Vector3(
      Math.sin(this.cameraYaw) * cosPitch,
      Math.sin(this.cameraPitch),
      Math.cos(this.cameraYaw) * cosPitch
    );
    const lookTarget = this.camera.position.clone().add(forward);
    this.camera.lookAt(lookTarget);
  }

  private updateLighting() {
    if (!this.timeState) {
      return;
    }
    const { sunPosition, ambientIntensity, sunLightIntensity } = this.timeState;
    this.directional.position.set(sunPosition.x, sunPosition.y, sunPosition.z);
    this.directional.target.position.set(0, 0, 0);
    this.directional.intensity = sunLightIntensity;
    this.ambient.intensity = ambientIntensity;
    const blend = THREE.MathUtils.clamp((ambientIntensity - 0.2) / 0.8, 0, 1);
    this.skyColor.copy(this.skyNight).lerp(this.skyDay, blend);
    this.scene.background = this.skyColor;
  }

  private handleKeyDown = (event: KeyboardEvent) => {
    if (this.shouldIgnoreKeyEvent(event)) {
      return;
    }
    this.keyState.add(event.code);
  };

  private handleKeyUp = (event: KeyboardEvent) => {
    this.keyState.delete(event.code);
  };

  private shouldIgnoreKeyEvent(event: KeyboardEvent): boolean {
    const target = event.target as HTMLElement | null;
    if (!target) {
      return false;
    }
    if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
      return true;
    }
    return Boolean(target.getAttribute('contenteditable'));
  }
}
