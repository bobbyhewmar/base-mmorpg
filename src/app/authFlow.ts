import { ApiClientError } from '../online/client';
import type { RegisterResponse } from '../online/contracts';
import type { PreGameEvent } from './preGameMachine';

export const resolveRegisterSuccessEvent = (response: RegisterResponse, login: string): PreGameEvent => {
  if (response.registration_state === 'created_pending_verification') {
    return {
      type: 'register_requires_verification',
      login,
    };
  }

  return {
    type: 'open_login',
  };
};

export const resolveLoginFailureEvent = (error: unknown, login: string, fallbackMessage: string): PreGameEvent => {
  if (error instanceof ApiClientError && error.reasonCode === 'auth.account_unverified') {
    return {
      type: 'register_requires_verification',
      login,
    };
  }

  return {
    type: 'auth_failed',
    message: fallbackMessage,
  };
};
