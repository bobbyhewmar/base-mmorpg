import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { selectAnimationClip, stripBoneScaleTracks } from './characterModelAssets';

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
});
