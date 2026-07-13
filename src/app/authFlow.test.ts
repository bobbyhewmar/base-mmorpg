import { describe, expect, it } from 'vitest';

import { ApiClientError } from '../online/client';
import { resolveLoginFailureEvent, resolveRegisterSuccessEvent } from './authFlow';

describe('auth flow helpers', () => {
  it('maps created_pending_verification to explicit pending verification state', () => {
    const event = resolveRegisterSuccessEvent(
      {
        account_id: 'acc_1',
        registration_state: 'created_pending_verification',
        next_step: 'login_or_verify',
      },
      'pending-user@example.com',
    );

    expect(event).toEqual({
      type: 'register_requires_verification',
      login: 'pending-user@example.com',
    });
  });

  it('maps auth.account_unverified to pending verification flow', () => {
    const event = resolveLoginFailureEvent(
      new ApiClientError('Account verification is still pending.', 'auth.account_unverified', 403),
      'pending-user@example.com',
      'fallback',
    );

    expect(event).toEqual({
      type: 'register_requires_verification',
      login: 'pending-user@example.com',
    });
  });
});
