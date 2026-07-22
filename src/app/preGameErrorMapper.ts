import { ApiClientError } from '../online/client';

export type PreGameErrorIntent = 'auth' | 'character_create' | 'world_enter' | 'attach';

type ReasonCodeCarrier = {
  reasonCode: string;
};

const isReasonCodeCarrier = (value: unknown): value is ReasonCodeCarrier => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  return typeof (value as Partial<ReasonCodeCarrier>).reasonCode === 'string';
};

const genericMessageByIntent: Record<PreGameErrorIntent, string> = {
  auth: 'Unable to complete this request right now. Please try again.',
  character_create: 'Unable to create the character right now. Please try again.',
  world_enter: 'Unable to enter the world right now. Please try again.',
  attach: 'Unable to complete the connection right now. Please try again.',
};

export const mapPreGameReasonCodeToUserMessage = (
  reasonCode: string | null | undefined,
  intent: PreGameErrorIntent,
): string => {
  switch (reasonCode) {
    case 'auth.invalid_credentials':
      return 'Incorrect login or password.';
    case 'auth.account_unverified':
      return 'Account verification is still pending.';
    case 'auth.account_locked':
      return 'This account is currently unavailable.';
    case 'auth.rate_limited':
      return 'Too many attempts. Please wait a moment and try again.';
    case 'auth.invalid_email':
      return 'Enter a valid email address.';
    case 'auth.email_unavailable':
      return 'This email is already in use.';
    case 'auth.social_provider_unsupported':
    case 'auth.social_not_configured':
    case 'auth.social_unavailable':
      return 'Social sign-in is not available right now.';
    case 'character.name_unavailable':
      return 'That character name is not available.';
    default:
      return genericMessageByIntent[intent];
  }
};

export const mapPreGameErrorToUserMessage = (error: unknown, intent: PreGameErrorIntent): string => {
  if (error instanceof ApiClientError) {
    return mapPreGameReasonCodeToUserMessage(error.reasonCode, intent);
  }
  if (isReasonCodeCarrier(error)) {
    return mapPreGameReasonCodeToUserMessage(error.reasonCode, intent);
  }
  return genericMessageByIntent[intent];
};
