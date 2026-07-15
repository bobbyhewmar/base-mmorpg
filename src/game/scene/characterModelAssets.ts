import * as THREE from 'three';
import { FBXLoader } from 'three/examples/jsm/loaders/FBXLoader.js';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';
import { getBaseClassDefinition } from '../data/characterClasses';
import type { BaseClass, CharacterSex } from '../domain/types';

type ClassCharacterModelActionName = 'idle' | 'walk' | 'run';

type ClassCharacterModelRuntime = {
  key: string;
  container: THREE.Group;
  root: THREE.Group | null;
  mixer: THREE.AnimationMixer | null;
  actions: Partial<Record<ClassCharacterModelActionName, THREE.AnimationAction>>;
  currentAction: ClassCharacterModelActionName | null;
  proceduralRoot?: THREE.Object3D;
};

export type ClassCharacterModelAppearance = {
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: number;
  hairColor: string;
  skinType: number;
};

const MODEL_RUNTIME_KEY = 'classCharacterModelRuntime';
const fbxLoader = new FBXLoader();
const gltfLoader = new GLTFLoader();
const textureLoader = new THREE.TextureLoader();
const textureCache = new Map<string, Promise<THREE.Texture>>();
const animationCache = new Map<string, Promise<THREE.AnimationClip | null>>();

const appearanceOptionIndex = (value: number): 0 | 1 | 2 => {
  if (value === 0 || value === 1 || value === 2) {
    return value;
  }
  return 0;
};

const modelKeyFor = (appearance: ClassCharacterModelAppearance, desiredHeight: number): string =>
  `${appearance.baseClass}:${appearance.sex}:${appearanceOptionIndex(appearance.hairStyle)}:${appearance.hairColor}:${appearanceOptionIndex(appearance.skinType)}:${desiredHeight.toFixed(2)}`;

const disposeMaterial = (material: THREE.Material | THREE.Material[]): void => {
  if (Array.isArray(material)) {
    material.forEach((entry) => entry.dispose());
    return;
  }
  material.dispose();
};

const disposeObjectTree = (object: THREE.Object3D): void => {
  object.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    mesh.geometry?.dispose();
    if (mesh.material) {
      disposeMaterial(mesh.material);
    }
  });
};

const loadTexture = (url: string): Promise<THREE.Texture> => {
  const cached = textureCache.get(url);
  if (cached) {
    return cached;
  }
  const promise = textureLoader.loadAsync(url).then((texture) => {
    texture.colorSpace = THREE.SRGBColorSpace;
    texture.wrapS = THREE.ClampToEdgeWrapping;
    texture.wrapT = THREE.ClampToEdgeWrapping;
    texture.needsUpdate = true;
    return texture;
  });
  textureCache.set(url, promise);
  return promise;
};

export const selectAnimationClip = (clips: THREE.AnimationClip[], preferredName: string): THREE.AnimationClip | null => {
  const normalizedPreferredName = preferredName.toLowerCase();
  return (
    clips.find((clip) => clip.name.toLowerCase() === normalizedPreferredName) ??
    clips.find((clip) => clip.name.toLowerCase().endsWith(`|${normalizedPreferredName}`)) ??
    clips.find((clip) => clip.name.toLowerCase().includes(normalizedPreferredName)) ??
    null
  );
};

export const stripBoneScaleTracks = (clip: THREE.AnimationClip): THREE.AnimationClip => {
  const tracks = clip.tracks.filter((track) => !track.name.toLowerCase().endsWith('.scale'));
  return new THREE.AnimationClip(clip.name, clip.duration, tracks, clip.blendMode);
};

const loadFbxAnimation = (url: string, preferredName: string): Promise<THREE.AnimationClip | null> => {
  const cacheKey = `fbx:${url}#${preferredName}`;
  const cached = animationCache.get(cacheKey);
  if (cached) {
    return cached;
  }
  const promise = fbxLoader.loadAsync(url).then((asset) => selectAnimationClip(asset.animations, preferredName));
  animationCache.set(cacheKey, promise);
  return promise;
};

const loadGltfAnimation = (url: string, preferredName: string): Promise<THREE.AnimationClip | null> => {
  const cacheKey = `gltf:${url}#${preferredName}`;
  const cached = animationCache.get(cacheKey);
  if (cached) {
    return cached;
  }
  const promise = gltfLoader
    .loadAsync(url)
    .then((asset) => {
      const clip = selectAnimationClip(asset.animations, preferredName);
      return clip ? stripBoneScaleTracks(clip) : null;
    });
  animationCache.set(cacheKey, promise);
  return promise;
};

const fitModelToHeight = (root: THREE.Object3D, desiredHeight: number): void => {
  root.updateMatrixWorld(true);
  const box = new THREE.Box3().setFromObject(root);
  const size = box.getSize(new THREE.Vector3());
  if (size.y <= 0.0001) {
    return;
  }

  root.scale.multiplyScalar(desiredHeight / size.y);
  root.updateMatrixWorld(true);

  const fittedBox = new THREE.Box3().setFromObject(root);
  const center = fittedBox.getCenter(new THREE.Vector3());
  root.position.x -= center.x;
  root.position.z -= center.z;
  root.position.y -= fittedBox.min.y;
};

const prepareLoadedModel = (
  root: THREE.Group,
  options: { castShadow: boolean; receiveShadow: boolean },
): void => {
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    mesh.castShadow = options.castShadow;
    mesh.receiveShadow = options.receiveShadow;
    if (mesh.material) {
      mesh.material = Array.isArray(mesh.material)
        ? mesh.material.map((material) => material.clone())
        : mesh.material.clone();
    }
  });
};

const materialListFor = (material: THREE.Material | THREE.Material[] | undefined): THREE.Material[] => {
  if (!material) {
    return [];
  }
  return Array.isArray(material) ? material : [material];
};

const applyHairColor = (root: THREE.Object3D, color: string): void => {
  const tint = new THREE.Color(color);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh || !mesh.material) {
      return;
    }
    const meshName = mesh.name.toLowerCase();
    for (const material of materialListFor(mesh.material)) {
      const materialName = material.name.toLowerCase();
      if (!meshName.includes('hair') && !materialName.includes('hair')) {
        continue;
      }
      if ('color' in material && material.color instanceof THREE.Color) {
        material.color.copy(tint);
        material.transparent = false;
        material.opacity = 1;
        material.depthWrite = true;
        material.needsUpdate = true;
      }
    }
  });
};

const firstSkinnedMesh = (root: THREE.Object3D): THREE.SkinnedMesh | null => {
  let result: THREE.SkinnedMesh | null = null;
  root.traverse((entry) => {
    if (result) {
      return;
    }
    const skinned = entry as THREE.SkinnedMesh;
    if (skinned.isSkinnedMesh) {
      result = skinned;
    }
  });
  return result;
};

const attachRiggedHairToBaseSkeleton = (baseRoot: THREE.Group, hairRoot: THREE.Group): void => {
  const baseSkinnedMesh = firstSkinnedMesh(baseRoot);
  const hairSkinnedMesh = firstSkinnedMesh(hairRoot);
  if (!baseSkinnedMesh || !hairSkinnedMesh) {
    throw new Error('Canonical hair asset must contain a skinned mesh compatible with the base body skeleton.');
  }

  baseRoot.updateMatrixWorld(true);
  hairRoot.updateMatrixWorld(true);
  hairSkinnedMesh.updateMatrixWorld(true);

  const material = Array.isArray(hairSkinnedMesh.material)
    ? hairSkinnedMesh.material.map((entry) => entry.clone())
    : hairSkinnedMesh.material.clone();
  const hair = new THREE.SkinnedMesh(hairSkinnedMesh.geometry.clone(), material);
  hair.name = `CanonicalHair_${hairSkinnedMesh.name || 'mesh'}`;
  hair.castShadow = true;
  hair.receiveShadow = true;
  hair.matrixAutoUpdate = false;
  hair.matrix.copy(new THREE.Matrix4().copy(baseRoot.matrixWorld).invert().multiply(hairSkinnedMesh.matrixWorld));
  hair.bind(baseSkinnedMesh.skeleton, hairSkinnedMesh.bindMatrix.clone());
  baseRoot.add(hair);
};

const applyBaseCharacterSkin = (root: THREE.Object3D, tint: string): void => {
  const tintColor = new THREE.Color(tint);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    for (const material of materialListFor(mesh.material)) {
      if (!material.name.toLowerCase().includes('superhero')) {
        continue;
      }
      const skinnedMaterial = material as THREE.MeshStandardMaterial;
      skinnedMaterial.color.copy(tintColor);
      skinnedMaterial.transparent = false;
      skinnedMaterial.opacity = 1;
      skinnedMaterial.depthWrite = true;
      skinnedMaterial.needsUpdate = true;
    }
  });
};

const prepareTexturedFbxModel = (
  root: THREE.Group,
  texture: THREE.Texture,
  options: { castShadow: boolean; receiveShadow: boolean },
): void => {
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    mesh.castShadow = options.castShadow;
    mesh.receiveShadow = options.receiveShadow;
    mesh.material = new THREE.MeshStandardMaterial({
      map: texture,
      roughness: 0.82,
      metalness: 0.04,
    });
  });
};

const tintLoadedModelMaterials = (root: THREE.Object3D, tint: string): void => {
  const tintColor = new THREE.Color(tint);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh || !mesh.material) {
      return;
    }
    const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
    for (const material of materials) {
      const normalizedName = material.name.toLowerCase();
      if (
        normalizedName.includes('regular') ||
        normalizedName.includes('skin') ||
        normalizedName.includes('eye') ||
        normalizedName.includes('hair')
      ) {
        continue;
      }
      if ('color' in material && material.color instanceof THREE.Color) {
        material.color.multiply(tintColor);
      }
    }
  });
};

const removeExistingRuntime = (parent: THREE.Object3D): void => {
  const existing = parent.userData[MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (!existing) {
    return;
  }
  existing.mixer?.stopAllAction();
  existing.container.removeFromParent();
  disposeObjectTree(existing.container);
  delete parent.userData[MODEL_RUNTIME_KEY];
};

const playModelAction = (runtime: ClassCharacterModelRuntime, nextActionName: ClassCharacterModelActionName): void => {
  if (runtime.currentAction === nextActionName) {
    return;
  }
  const nextAction = runtime.actions[nextActionName];
  if (!nextAction) {
    runtime.currentAction = nextActionName;
    return;
  }
  const previousAction = runtime.currentAction ? runtime.actions[runtime.currentAction] : null;
  nextAction.reset().fadeIn(0.16).play();
  previousAction?.fadeOut(0.16);
  runtime.currentAction = nextActionName;
};

export const ensureClassCharacterModel = (
  parent: THREE.Object3D,
  appearance: ClassCharacterModelAppearance,
  options: {
    desiredHeight: number;
    castShadow?: boolean;
    receiveShadow?: boolean;
    proceduralRoot?: THREE.Object3D;
  },
): void => {
  const key = modelKeyFor(appearance, options.desiredHeight);
  const existing = parent.userData[MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (existing?.key === key) {
    return;
  }

  removeExistingRuntime(parent);
  if (options.proceduralRoot) {
    options.proceduralRoot.visible = false;
  }

  const definition = getBaseClassDefinition(appearance.baseClass);
  const container = new THREE.Group();
  container.visible = false;
  parent.add(container);

  const runtime: ClassCharacterModelRuntime = {
    key,
    container,
    root: null,
    mixer: null,
    actions: {},
    currentAction: null,
    proceduralRoot: options.proceduralRoot,
  };
  parent.userData[MODEL_RUNTIME_KEY] = runtime;

  const shadowOptions = {
    castShadow: options.castShadow ?? true,
    receiveShadow: options.receiveShadow ?? true,
  };

  const visual = definition.visual;
  let assetPromise: Promise<{
    model: THREE.Group;
    idleClip: THREE.AnimationClip | null;
    walkClip: THREE.AnimationClip | null;
    runClip: THREE.AnimationClip | null;
  }>;

  if (visual.kind === 'gltf_base_character') {
    const skinType = appearanceOptionIndex(appearance.skinType);
    const skinTint = visual.skinTints[appearance.sex][skinType] ?? '#ffffff';
    const hairUrl = visual.hairModelUrls[appearance.sex][appearanceOptionIndex(appearance.hairStyle)];
    if (!hairUrl) {
      throw new Error(`Missing canonical hair asset for ${appearance.sex} hairstyle ${appearance.hairStyle}.`);
    }
    assetPromise = Promise.all([
      gltfLoader.loadAsync(visual.modelUrls[appearance.sex]),
      gltfLoader.loadAsync(hairUrl),
      loadGltfAnimation(visual.animationUrl, visual.idleClipName),
      loadGltfAnimation(visual.animationUrl, visual.walkClipName),
      loadGltfAnimation(visual.animationUrl, visual.runClipName),
    ]).then(([modelAsset, hairAsset, idleClip, walkClip, runClip]) => {
      const model = modelAsset.scene;
      prepareLoadedModel(model, shadowOptions);
      applyBaseCharacterSkin(model, skinTint);
      prepareLoadedModel(hairAsset.scene, shadowOptions);
      attachRiggedHairToBaseSkeleton(model, hairAsset.scene);
      applyHairColor(model, appearance.hairColor);
      disposeObjectTree(hairAsset.scene);
      return { model, idleClip, walkClip, runClip };
    });
  } else {
    assetPromise = Promise.all([
      fbxLoader.loadAsync(visual.modelUrl),
      loadTexture(visual.textures[appearance.sex]),
      loadFbxAnimation(visual.idleAnimationUrl, 'Idle'),
      loadFbxAnimation(visual.runAnimationUrl, 'Run'),
    ]).then(([model, texture, idleClip, runClip]) => {
      prepareTexturedFbxModel(model, texture, shadowOptions);
      return { model, idleClip, walkClip: null, runClip };
    });
  }

  void assetPromise
    .then(({ model, idleClip, walkClip, runClip }) => {
      if (parent.userData[MODEL_RUNTIME_KEY] !== runtime) {
        disposeObjectTree(model);
        return;
      }
      fitModelToHeight(model, options.desiredHeight);
      container.add(model);
      runtime.root = model;
      runtime.mixer = new THREE.AnimationMixer(model);
      if (idleClip) {
        runtime.actions.idle = runtime.mixer.clipAction(idleClip);
      }
      if (walkClip) {
        runtime.actions.walk = runtime.mixer.clipAction(walkClip);
      }
      if (runClip) {
        runtime.actions.run = runtime.mixer.clipAction(runClip);
      }
      if (runtime.proceduralRoot) {
        runtime.proceduralRoot.visible = false;
      }
      container.visible = true;
      playModelAction(runtime, 'idle');
    })
    .catch((error: unknown) => {
      console.error('Failed to load canonical character model.', error);
    });
};

export const updateClassCharacterModelAnimation = (
  parent: THREE.Object3D,
  deltaMs: number,
  options: {
    moving?: boolean;
    dead?: boolean;
    casting?: boolean;
    basicAttacking?: boolean;
    movementMode?: 'run' | 'walk';
  } = {},
): void => {
  const runtime = parent.userData[MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (!runtime?.root) {
    return;
  }

  runtime.root.rotation.set(0, 0, 0);
  runtime.root.position.y = 0;
  if (options.dead) {
    runtime.root.rotation.z = Math.PI * 0.5;
    runtime.root.position.y = 0.56;
    playModelAction(runtime, 'idle');
  } else {
    playModelAction(runtime, options.moving ? options.movementMode ?? 'run' : 'idle');
    if (options.casting) {
      runtime.root.rotation.x = Math.sin(performance.now() * 0.012) * 0.08;
    }
    if (options.basicAttacking) {
      runtime.root.rotation.y = Math.sin(performance.now() * 0.018) * 0.18;
    }
  }

  runtime.mixer?.update(Math.max(0, deltaMs) / 1000);
};
