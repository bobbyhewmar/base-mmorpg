import * as THREE from 'three';
import type { BaseClass, CharacterRace, CharacterSex, CharacterSummary } from '../online/contracts';
import { animateCharacterVisual, createCharacterVisual } from './characterLobbyScene';

export interface CharacterCreationPreviewState {
  race: CharacterRace | null;
  baseClass: BaseClass | null;
  sex: CharacterSex | null;
  hairStyle: number | null;
  hairColor: string | null;
  skinType: number | null;
  baseClassOptions: BaseClass[];
  sexOptions: CharacterSex[];
}

type CreationVisual = {
  group: THREE.Group;
  baseY: number;
  baseRotationY: number;
};

const FOCUSED_CHARACTER_SCALE = 1.16;
const ROSTER_CHARACTER_SCALE = 1.05;

const createMaterial = (color: string, roughness = 0.86): THREE.MeshStandardMaterial =>
  new THREE.MeshStandardMaterial({ color, roughness });

const createBasicMaterial = (color: string, opacity = 1): THREE.MeshBasicMaterial =>
  new THREE.MeshBasicMaterial({
    color,
    transparent: opacity < 1,
    opacity,
    depthWrite: opacity >= 1,
    side: THREE.DoubleSide,
  });

const addFloor = (scene: THREE.Scene, color: string): void => {
  const floor = new THREE.Mesh(new THREE.PlaneGeometry(34, 22), createMaterial(color, 0.96));
  floor.rotation.x = -Math.PI / 2;
  floor.receiveShadow = true;
  scene.add(floor);
};

const createTree = (x: number, z: number, scale: number, foliage = '#28452f'): THREE.Group => {
  const tree = new THREE.Group();
  const trunk = new THREE.Mesh(new THREE.CylinderGeometry(0.22 * scale, 0.34 * scale, 3.7 * scale, 8), createMaterial('#4b3a28'));
  trunk.position.y = 1.85 * scale;
  const crown = new THREE.Mesh(new THREE.ConeGeometry(1.2 * scale, 3.2 * scale, 8), createMaterial(foliage));
  crown.position.y = 3.65 * scale;
  tree.add(trunk, crown);
  tree.position.set(x, 0, z);
  return tree;
};

const createRock = (x: number, z: number, scale: number, color = '#6d6860'): THREE.Mesh => {
  const rock = new THREE.Mesh(new THREE.DodecahedronGeometry(scale, 0), createMaterial(color, 0.98));
  rock.position.set(x, scale * 0.54, z);
  rock.rotation.set(0.2, x * 0.17, 0.1);
  rock.scale.y = 0.64;
  rock.castShadow = true;
  rock.receiveShadow = true;
  return rock;
};

const createTorch = (x: number, z: number): THREE.Group => {
  const torch = new THREE.Group();
  const post = new THREE.Mesh(new THREE.CylinderGeometry(0.07, 0.1, 1.7, 6), createMaterial('#4a3320', 0.9));
  post.position.y = 0.85;
  const flame = new THREE.Mesh(new THREE.ConeGeometry(0.23, 0.75, 7), createBasicMaterial('#ffae62', 0.94));
  flame.position.y = 2.05;
  const light = new THREE.PointLight('#ff9c55', 1.35, 7);
  light.position.y = 2.15;
  torch.add(post, flame, light);
  torch.position.set(x, 0, z);
  return torch;
};

const createHouse = (x: number, z: number, scale: number): THREE.Group => {
  const house = new THREE.Group();
  const body = new THREE.Mesh(new THREE.BoxGeometry(1.8 * scale, 1.05 * scale, 1.25 * scale), createMaterial('#c3b394', 0.92));
  body.position.y = 0.55 * scale;
  const roof = new THREE.Mesh(new THREE.ConeGeometry(1.35 * scale, 0.82 * scale, 4), createMaterial('#8b5e34', 0.94));
  roof.position.y = 1.42 * scale;
  roof.rotation.y = Math.PI / 4;
  house.add(body, roof);
  house.position.set(x, 0, z);
  return house;
};

const createWindmill = (x: number, z: number): THREE.Group => {
  const mill = new THREE.Group();
  const tower = new THREE.Mesh(new THREE.CylinderGeometry(0.42, 0.68, 2.9, 6), createMaterial('#b7a381', 0.94));
  tower.position.y = 1.45;
  const hub = new THREE.Mesh(new THREE.SphereGeometry(0.16, 8, 6), createMaterial('#6a4a2d', 0.8));
  hub.position.set(0, 2.7, 0.46);
  for (let index = 0; index < 4; index += 1) {
    const blade = new THREE.Mesh(new THREE.BoxGeometry(0.12, 1.1, 0.04), createMaterial('#d6c6a4', 0.78));
    blade.position.set(0, 2.7, 0.5);
    blade.rotation.z = (Math.PI / 2) * index;
    blade.translateY(0.5);
    mill.add(blade);
  }
  mill.add(tower, hub);
  mill.position.set(x, 0, z);
  return mill;
};

const createMonument = (x: number, z: number, color = '#5a5655'): THREE.Group => {
  const monument = new THREE.Group();
  const base = new THREE.Mesh(new THREE.BoxGeometry(2.2, 0.35, 0.85), createMaterial(color, 0.98));
  base.position.y = 0.18;
  const slab = new THREE.Mesh(new THREE.BoxGeometry(1.35, 3.25, 0.38), createMaterial(color, 0.98));
  slab.position.y = 1.93;
  const crest = new THREE.Mesh(new THREE.TorusGeometry(0.42, 0.035, 8, 28), createMaterial('#9c8b64', 0.72));
  crest.position.set(0, 2.65, 0.22);
  monument.add(base, slab, crest);
  monument.position.set(x, 0, z);
  return monument;
};

const createWaterfall = (x: number, z: number, height: number): THREE.Group => {
  const group = new THREE.Group();
  for (let index = 0; index < 4; index += 1) {
    const fall = new THREE.Mesh(
      new THREE.PlaneGeometry(1.05, height + index * 0.3),
      createBasicMaterial('#d7f7ff', 0.34),
    );
    fall.position.set(x + index * 0.8, height / 2, z - index * 0.05);
    fall.rotation.x = -0.03;
    group.add(fall);
  }
  return group;
};

const createMineTower = (x: number, z: number): THREE.Group => {
  const group = new THREE.Group();
  const frameMaterial = createMaterial('#6d4f34', 0.92);
  for (const offset of [-0.42, 0.42]) {
    const support = new THREE.Mesh(new THREE.BoxGeometry(0.16, 3.6, 0.16), frameMaterial);
    support.position.set(offset, 1.8, 0);
    support.rotation.z = offset * 0.12;
    group.add(support);
  }
  const bridge = new THREE.Mesh(new THREE.BoxGeometry(1.9, 0.18, 0.28), frameMaterial);
  bridge.position.y = 2.9;
  const roof = new THREE.Mesh(new THREE.ConeGeometry(0.9, 0.7, 4), createMaterial('#7e6045', 0.9));
  roof.position.y = 3.55;
  roof.rotation.y = Math.PI / 4;
  group.add(bridge, roof);
  group.position.set(x, 0, z);
  return group;
};

const previewCharacter = (
  race: CharacterRace,
  baseClass: BaseClass,
  sex: CharacterSex,
  hairStyle: number,
  hairColor: string,
  skinType: number,
  index: number,
): CharacterSummary => ({
  character_id: `creation_${race}_${baseClass}_${sex}_${index}`,
  name: '',
  race,
  base_class: baseClass,
  sex,
  hair_style: hairStyle,
  hair_color: hairColor,
  skin_type: skinType,
  level: 1,
  last_region_id: 'character_creation',
  is_enterable: true,
});

export class CharacterCreationScene {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(40, 1, 0.1, 120);
  private readonly visuals: CreationVisual[] = [];
  private frameHandle = 0;
  private destroyed = false;

  constructor(
    private readonly host: HTMLElement,
    private readonly preview: CharacterCreationPreviewState,
  ) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.host.replaceChildren(this.renderer.domElement);
    this.configureScene();
    window.addEventListener('resize', this.handleResize);
    this.handleResize();
    this.animate(performance.now());
  }

  destroy(): void {
    this.destroyed = true;
    cancelAnimationFrame(this.frameHandle);
    window.removeEventListener('resize', this.handleResize);
    this.scene.traverse((object) => {
      const mesh = object as THREE.Mesh;
      mesh.geometry?.dispose();
      const material = mesh.material;
      if (Array.isArray(material)) {
        material.forEach((entry) => entry.dispose());
      } else {
        material?.dispose();
      }
    });
    this.renderer.dispose();
    this.host.replaceChildren();
  }

  private configureScene(): void {
    this.renderer.setClearAlpha(this.preview.race ? 1 : 0);
    const focused = Boolean(this.preview.baseClass && this.preview.sex);
    this.camera.position.set(0, focused ? 2.7 : 3.05, focused ? 8.4 : 9.2);
    this.camera.lookAt(focused ? 0.52 : 0, focused ? 1.55 : 1.52, 0);
    this.addLighting();
    this.addEnvironment();
    this.addCharacters();
  }

  private addLighting(): void {
    const ambient = new THREE.HemisphereLight('#e6e0c8', '#171511', 1.22);
    this.scene.add(ambient);
    const sun = new THREE.DirectionalLight('#ffe5b8', 2.25);
    sun.position.set(5.5, 9.8, 5.8);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    this.scene.add(sun);
  }

  private addEnvironment(): void {
    switch (this.preview.race) {
      case 'Human':
        this.createHumanVillage();
        return;
      case 'Elf':
        this.createElfFalls();
        return;
      case 'Dark Elf':
        this.createDarkElfRuins();
        return;
      case 'Orc':
        this.createOrcHold();
        return;
      case 'Dwarf':
        this.createDwarfValley();
        return;
      default:
        this.createForestIdle();
    }
  }

  private createForestIdle(): void {
    this.scene.fog = new THREE.Fog('#10251d', 16, 36);
  }

  private createHumanVillage(): void {
    this.scene.background = new THREE.Color('#8ac4bf');
    this.scene.fog = new THREE.Fog('#8ac4bf', 18, 44);
    addFloor(this.scene, '#58724f');
    this.scene.add(createHouse(-6.4, -4.4, 1.2), createHouse(5.8, -4.7, 1.05), createHouse(1.4, -6.3, 0.92));
    this.scene.add(createWindmill(-1.9, -7.2));
    this.scene.add(createTree(-8.5, -1.3, 0.9, '#496b3f'), createTree(8.1, -2.2, 1.1, '#496b3f'));
    this.scene.add(createRock(-5.5, 1.8, 0.65), createRock(4.7, 1.7, 0.55));
  }

  private createElfFalls(): void {
    this.scene.background = new THREE.Color('#7cc6d3');
    this.scene.fog = new THREE.Fog('#c9eef4', 14, 38);
    addFloor(this.scene, '#5f7a55');
    const moon = new THREE.Mesh(new THREE.SphereGeometry(1.25, 24, 12), createBasicMaterial('#eef8f2', 0.82));
    moon.position.set(4.7, 6.4, -10.5);
    this.scene.add(moon);
    this.scene.add(createWaterfall(-2.1, -8.4, 6.8), createWaterfall(2.0, -8.6, 5.6));
    this.scene.add(createMonument(0.1, -7.2, '#d7d6cd'));
    this.scene.add(createTree(-7.6, -1.8, 0.96, '#657a47'), createTree(7.6, -2.3, 0.9, '#657a47'));
    this.scene.add(createRock(-4.9, 1.3, 0.72, '#8d8c80'), createRock(5.2, 1.1, 0.8, '#8d8c80'));
  }

  private createDarkElfRuins(): void {
    this.scene.background = new THREE.Color('#886f5d');
    this.scene.fog = new THREE.Fog('#6c5c55', 15, 40);
    addFloor(this.scene, '#4b493e');
    for (const [x, z, scale] of [
      [-8.2, -9.2, 3.7],
      [-3.7, -10.5, 4.9],
      [4.2, -9.9, 4.2],
      [8.8, -8.5, 3.4],
    ] as const) {
      const mountain = new THREE.Mesh(new THREE.ConeGeometry(scale, scale * 2.1, 6), createMaterial('#5d5a53', 1));
      mountain.position.set(x, scale, z);
      this.scene.add(mountain);
    }
    this.scene.add(createMonument(0, -6.5, '#353238'));
    this.scene.add(createTree(-7.7, -2.2, 0.62, '#1f211d'), createTree(7.4, -2.6, 0.65, '#1f211d'));
    this.scene.add(createRock(-4.5, 1.4, 0.86, '#555149'), createRock(5.6, 1.0, 0.74, '#555149'));
  }

  private createOrcHold(): void {
    this.scene.background = new THREE.Color('#8f9582');
    this.scene.fog = new THREE.Fog('#807c6f', 16, 42);
    addFloor(this.scene, '#5a5c42');
    for (const [x, z, scale] of [
      [-7.9, -9, 4.3],
      [-2.7, -10.6, 5.2],
      [4.6, -9.2, 4.5],
    ] as const) {
      const mountain = new THREE.Mesh(new THREE.ConeGeometry(scale, scale * 2.2, 7), createMaterial('#77736a', 1));
      mountain.position.set(x, scale, z);
      this.scene.add(mountain);
    }
    const fortress = new THREE.Group();
    const body = new THREE.Mesh(new THREE.BoxGeometry(5.2, 2.2, 1.3), createMaterial('#6d675b', 0.98));
    body.position.y = 1.1;
    const crown = new THREE.Mesh(new THREE.BoxGeometry(4.0, 0.65, 1.55), createMaterial('#7c7467', 0.98));
    crown.position.y = 2.45;
    fortress.add(body, crown);
    fortress.position.set(0, 0, -7.0);
    this.scene.add(fortress, createTorch(-3.4, -4.9), createTorch(3.4, -4.9));
    this.scene.add(createTree(-8.3, -2.1, 0.8, '#243728'), createTree(7.9, -2.3, 0.82, '#243728'));
    this.scene.add(createRock(-5.8, 1.6, 0.84, '#756e61'), createRock(5.2, 1.4, 0.72, '#756e61'));
  }

  private createDwarfValley(): void {
    this.scene.background = new THREE.Color('#9dc9ca');
    this.scene.fog = new THREE.Fog('#c9d2cf', 17, 43);
    addFloor(this.scene, '#d4d3ca');
    for (const [x, z, scale] of [
      [-8.2, -9.2, 4.1],
      [-2.7, -10.6, 5.3],
      [4.7, -9.4, 4.4],
    ] as const) {
      const mountain = new THREE.Mesh(new THREE.ConeGeometry(scale, scale * 2.4, 7), createMaterial('#9b9c99', 1));
      mountain.position.set(x, scale, z);
      this.scene.add(mountain);
    }
    this.scene.add(createMineTower(0, -6.7));
    this.scene.add(createHouse(-5.5, -4.5, 0.86), createHouse(5.8, -4.4, 0.82));
    this.scene.add(createRock(-4.4, 1.6, 0.82, '#a8a8a0'), createRock(4.7, 1.5, 0.7, '#a8a8a0'));
  }

  private addCharacters(): void {
    const race = this.preview.race;
    const hairStyle = this.preview.hairStyle;
    const hairColor = this.preview.hairColor;
    const skinType = this.preview.skinType;
    if (!race) {
      return;
    }
    if (hairStyle === null || !hairColor || skinType === null) {
      return;
    }
    const focused = this.preview.baseClass && this.preview.sex;
    const variants = focused
      ? [{ baseClass: this.preview.baseClass as BaseClass, sex: this.preview.sex as CharacterSex }]
      : this.resolveRosterVariants();
    const positions = focused
      ? [new THREE.Vector3(0.85, 0, -0.15)]
      : [
          new THREE.Vector3(-5.2, 0, 0.45),
          new THREE.Vector3(-1.75, 0, 0.0),
          new THREE.Vector3(1.75, 0, 0.0),
          new THREE.Vector3(5.2, 0, 0.45),
        ];

    variants.forEach((variant, index) => {
      const group = createCharacterVisual(
        previewCharacter(
          race as CharacterRace,
          variant.baseClass,
          variant.sex,
          hairStyle,
          hairColor,
          skinType,
          index,
        ),
        { showName: false },
      );
      group.position.copy(positions[index] ?? positions[positions.length - 1]);
      group.scale.multiplyScalar(focused ? FOCUSED_CHARACTER_SCALE : ROSTER_CHARACTER_SCALE);
      group.rotation.y = focused ? -0.18 : index < 2 ? -0.12 : 0.12;
      group.traverse((object) => {
        const mesh = object as THREE.Mesh;
        if (mesh.isMesh) {
          mesh.castShadow = true;
          mesh.receiveShadow = true;
        }
      });
      this.visuals.push({ group, baseY: group.position.y, baseRotationY: group.rotation.y });
      this.scene.add(group);
    });
  }

  private resolveRosterVariants(): Array<{ baseClass: BaseClass; sex: CharacterSex }> {
    const baseClasses = this.preview.baseClassOptions;
    const sexOptions = this.preview.sexOptions;
    const defaultSex = this.preview.sex ?? sexOptions[0] ?? 'Male';
    return baseClasses.slice(0, 4).map((baseClass) => ({ baseClass, sex: defaultSex }));
  }

  private readonly handleResize = (): void => {
    const width = Math.max(this.host.clientWidth, 1);
    const height = Math.max(this.host.clientHeight, 1);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height, false);
  };

  private animate = (now: number): void => {
    if (this.destroyed) {
      return;
    }
    this.visuals.forEach((visual, index) => {
      const idle = Math.sin(now * 0.0018 + index * 0.55);
      visual.group.position.y = visual.baseY + idle * 0.025;
      visual.group.rotation.y = visual.baseRotationY + idle * 0.035;
      animateCharacterVisual(visual.group, now, { phaseOffset: index * 0.58 });
    });
    this.renderer.render(this.scene, this.camera);
    this.frameHandle = requestAnimationFrame(this.animate);
  };
}
