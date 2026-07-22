import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import {
  CLASS_CHARACTER_MODEL_RUNTIME_KEY,
  pickLobbyInteractionClip,
  selectAnimationClip,
  selectLobbyInteractionClips,
  stripBoneScaleTracks,
  triggerClassCharacterModelLobbyInteraction,
} from './characterModelAssets';

describe('character model assets', () => {
  it('selects the named animation instead of the first targeting pose clip', () => {
    const targetingPose = new THREE.AnimationClip('Root|0.Targeting Pose', 0.04, []);
    const run = new THREE.AnimationClip('Root|Run', 0.66, []);

    expect(selectAnimationClip([targetingPose, run], 'Run')).toBe(run);
  });

  it('selects idle by suffix from exported FBX animation names', () => {
    const idle = new THREE.AnimationClip('Root|Idle', 1.06, []);

    expect(selectAnimationClip([idle], 'Idle')).toBe(idle);
  });

  it('selects exact Universal Animation Library clip names', () => {
    const idle = new THREE.AnimationClip('Idle_Loop', 1.2, []);
    const jog = new THREE.AnimationClip('Jog_Fwd_Loop', 0.8, []);

    expect(selectAnimationClip([idle, jog], 'Jog_Fwd_Loop')).toBe(jog);
  });

  it('strips scale tracks from reusable humanoid clips to preserve body bind proportions', () => {
    const clip = new THREE.AnimationClip('Jog_Fwd_Loop', 0.8, [
      new THREE.VectorKeyframeTrack('pelvis.position', [0], [0, 0, 0]),
      new THREE.QuaternionKeyframeTrack('pelvis.quaternion', [0], [0, 0, 0, 1]),
      new THREE.VectorKeyframeTrack('pelvis.scale', [0], [1.2, 0.7, 1]),
    ]);

    const sanitized = stripBoneScaleTracks(clip);

    expect(sanitized.tracks.map((track) => track.name)).toEqual(['pelvis.position', 'pelvis.quaternion']);
  });

  it('filters lobby interaction clips without including locomotion equivalents', () => {
    const idle = new THREE.AnimationClip('Idle_Loop', 1.2, []);
    const walk = new THREE.AnimationClip('Walk_Loop', 0.8, []);
    const jog = new THREE.AnimationClip('Jog_Fwd_Loop', 0.8, []);
    const wave = new THREE.AnimationClip('Wave', 0.9, []);
    const celebrate = new THREE.AnimationClip('Victory_Cheer', 1.4, []);
    const turnLeft = new THREE.AnimationClip('Turn_Left', 0.7, []);

    const clips = selectLobbyInteractionClips([idle, walk, jog, wave, celebrate, turnLeft], [
      'Idle_Loop',
      'Walk_Loop',
      'Jog_Fwd_Loop',
    ]);

    expect(clips).toEqual([wave, celebrate]);
  });

  it('picks any eligible lobby interaction clip by random slice', () => {
    const wave = new THREE.AnimationClip('Wave', 0.9, []);
    const cheer = new THREE.AnimationClip('Cheer', 1.4, []);
    const salute = new THREE.AnimationClip('Salute', 1.1, []);

    expect(pickLobbyInteractionClip([wave, cheer, salute], 0)).toBe(wave);
    expect(pickLobbyInteractionClip([wave, cheer, salute], 0.5)).toBe(cheer);
    expect(pickLobbyInteractionClip([wave, cheer, salute], 0.99)).toBe(salute);
  });

  it('falls back to idle when no eligible lobby clip exists', () => {
    const parent = new THREE.Group();
    const idleClip = new THREE.AnimationClip('Idle_Loop', 1.2, []);
    const idleAction = {
      getClip: () => idleClip,
      reset() {
        return this;
      },
      fadeIn() {
        return this;
      },
      play() {
        return this;
      },
    };
    parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] = {
      root: new THREE.Group(),
      mixer: {},
      actions: { idle: idleAction },
      currentAction: 'idle',
      lobbyInteractionActions: [],
      activeLobbyInteractionAction: null,
    };

    const started = triggerClassCharacterModelLobbyInteraction(parent, 0.42);

    expect(started).toBe(false);
    expect(parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY].currentAction).toBe('idle');
  });

  it('triggers a real lobby interaction clip when one is eligible', () => {
    const parent = new THREE.Group();
    const idleClip = new THREE.AnimationClip('Idle_Loop', 1.2, []);
    const waveClip = new THREE.AnimationClip('Wave', 0.9, []);
    let playedWave = false;
    const idleAction = {
      getClip: () => idleClip,
      fadeOut() {
        return this;
      },
    };
    const waveAction = {
      getClip: () => waveClip,
      reset() {
        return this;
      },
      setLoop() {
        return this;
      },
      fadeIn() {
        return this;
      },
      play() {
        playedWave = true;
        return this;
      },
      stop() {
        return this;
      },
    };
    parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] = {
      root: new THREE.Group(),
      mixer: {
        clipAction: () => waveAction,
      },
      actions: { idle: idleAction },
      currentAction: 'idle',
      lobbyInteractionActions: [waveAction],
      activeLobbyInteractionAction: null,
    };

    const started = triggerClassCharacterModelLobbyInteraction(parent, 0);

    expect(started).toBe(true);
    expect(playedWave).toBe(true);
    expect(parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY].activeLobbyInteractionAction).toBe(waveAction);
  });
});
