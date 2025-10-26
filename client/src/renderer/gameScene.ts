import * as THREE from 'three';
import type { ChunkDeltaEvent, ChunkSummaryEvent } from '@shared/protocol';

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

  constructor(private canvas: HTMLCanvasElement) {
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color('#0f172a');

    const aspect = canvas.clientWidth / Math.max(canvas.clientHeight, 1);
    this.camera = new THREE.PerspectiveCamera(60, aspect, 0.1, 10_000);
    this.camera.position.set(0, 400, 700);
    this.camera.lookAt(new THREE.Vector3(0, 0, 0));

    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
    this.renderer.setPixelRatio(window.devicePixelRatio);
    this.renderer.setSize(canvas.clientWidth, canvas.clientHeight);

    this.ambient = new THREE.AmbientLight(0xffffff, 0.6);
    this.directional = new THREE.DirectionalLight(0xffffff, 0.8);
    this.directional.position.set(400, 800, 400);

    this.scene.add(this.ambient);
    this.scene.add(this.directional);

    const grid = new THREE.GridHelper(2000, 40, '#22d3ee', '#1e293b');
    this.scene.add(grid);

    window.addEventListener('resize', () => this.handleResize());
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
      this.renderer.render(this.scene, this.camera);
    };
    renderLoop();
  }

  private handleResize() {
    const width = this.canvas.clientWidth;
    const height = Math.max(this.canvas.clientHeight, 1);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
  }
}
