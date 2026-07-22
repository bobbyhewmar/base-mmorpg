import { describe, expect, it } from 'vitest';

import { ApiClientError } from '../online/client';
import { mapPreGameErrorToUserMessage, mapPreGameReasonCodeToUserMessage } from './preGameErrorMapper';

describe('pre-game error mapper', () => {
  it('maps auth.invalid_credentials to a friendly login message', () => {
    const message = mapPreGameErrorToUserMessage(
      new ApiClientError('Invalid login or password.', 'auth.invalid_credentials', 401),
      'auth',
    );

    expect(message).toBe('Incorrect login or password.');
    expect(message).not.toContain('auth.invalid_credentials');
  });

  it('falls back to a generic safe auth message for unknown errors', () => {
    const message = mapPreGameErrorToUserMessage(
      new ApiClientError('Database timeout while validating auth.', 'system.database_timeout', 500),
      'auth',
    );

    expect(message).toBe('Unable to complete this request right now. Please try again.');
    expect(message).not.toContain('system.database_timeout');
    expect(message).not.toContain('Database timeout');
  });

  it('falls back to a generic safe auth message for network failures', () => {
    const message = mapPreGameErrorToUserMessage(new TypeError('Failed to fetch'), 'auth');

    expect(message).toBe('Unable to complete this request right now. Please try again.');
    expect(message).not.toContain('Failed to fetch');
  });

  it('maps auth.email_unavailable to a friendly registration message', () => {
    const message = mapPreGameErrorToUserMessage(
      new ApiClientError('Email is unavailable.', 'auth.email_unavailable', 409),
      'auth',
    );

    expect(message).toBe('This email is already in use.');
    expect(message).not.toContain('auth.email_unavailable');
  });

  it('maps auth.social_not_configured to a safe social-auth message', () => {
    const message = mapPreGameErrorToUserMessage(
      new ApiClientError('Social sign-in is not configured.', 'auth.social_not_configured', 503),
      'auth',
    );

    expect(message).toBe('Social sign-in is not available right now.');
    expect(message).not.toContain('auth.social_not_configured');
  });

  it('maps character.name_unavailable to the shared character-creation copy', () => {
    const message = mapPreGameReasonCodeToUserMessage('character.name_unavailable', 'character_create');

    expect(message).toBe('That character name is not available.');
    expect(message).not.toContain('character.name_unavailable');
  });
});
