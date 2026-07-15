import { describe, expect, it } from 'vitest';

import { resolveCharacterCreationOptions } from './characterCreationOptions';

const appearanceOptions = {
  hair_styles: [0, 1, 2],
  hair_color_default: '#6b4e37',
  skin_types: [0, 1, 2],
};

describe('character creation options', () => {
  it('derives options from the selected race and marks the first canonical choices by default', () => {
    const options = resolveCharacterCreationOptions(
      {
        races: [
          {
            race: 'Dwarf',
            enabled: true,
            base_classes: ['Fighter'],
            sex_options: ['Male', 'Female'],
            appearance_options: appearanceOptions,
          },
          {
            race: 'Human',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Female'],
            appearance_options: appearanceOptions,
          },
        ],
      },
      'Human',
    );

    expect(options.selectedRace).toBe('Human');
    expect(options.baseClassOptions).toEqual(['Fighter', 'Mage']);
    expect(options.sexOptions).toEqual(['Female']);
    expect(options.selectedBaseClass).toBe('Fighter');
    expect(options.selectedSex).toBe('Female');
    expect(options.selectedHairStyle).toBe(0);
    expect(options.selectedHairColor).toBe('#6b4e37');
    expect(options.selectedSkinType).toBe(0);
  });

  it('keeps race ordering deterministic and defaults to the first enabled race', () => {
    const options = resolveCharacterCreationOptions(
      {
        races: [
          {
            race: 'Orc',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Male', 'Female'],
            appearance_options: appearanceOptions,
          },
          {
            race: 'Dark Elf',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Male', 'Female'],
            appearance_options: appearanceOptions,
          },
        ],
      },
      null,
    );

    expect(options.raceOptions.map((race) => race.race)).toEqual(['Dark Elf', 'Orc']);
    expect(options.selectedRace).toBe('Dark Elf');
    expect(options.selectedBaseClass).toBe('Fighter');
    expect(options.selectedSex).toBe('Male');
  });

  it('preserves explicit hairstyle, hair color, and skin type choices', () => {
    const options = resolveCharacterCreationOptions(
      {
        races: [
          {
            race: 'Human',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Male', 'Female'],
            appearance_options: appearanceOptions,
          },
        ],
      },
      'Human',
      'Mage',
      'Female',
      2,
      1,
      '#aabbcc',
    );

    expect(options.selectedRace).toBe('Human');
    expect(options.selectedBaseClass).toBe('Mage');
    expect(options.selectedSex).toBe('Female');
    expect(options.selectedHairStyle).toBe(2);
    expect(options.selectedHairColor).toBe('#aabbcc');
    expect(options.selectedSkinType).toBe(1);
  });
});
