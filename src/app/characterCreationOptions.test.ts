import { describe, expect, it } from 'vitest';

import { resolveCharacterCreationOptions } from './characterCreationOptions';

const appearanceOptions = {
  hair_styles: [0, 1, 2],
  hair_colors: [0, 1, 2],
  faces: [0, 1, 2],
};

describe('character creation options', () => {
  it('derives base class and sex options from the currently selected race', () => {
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
  });

  it('keeps race ordering deterministic even when catalog order varies', () => {
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
    expect(options.selectedRace).toBeNull();
  });

  it('keeps class and sex blank until the player explicitly selects them', () => {
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
    );

    expect(options.selectedRace).toBe('Human');
    expect(options.selectedBaseClass).toBeNull();
    expect(options.selectedSex).toBeNull();
  });
});
