import type { ChatMessageServerMessage } from './contracts';

export const formatChatMessage = (
  message: ChatMessageServerMessage,
  characterId: string,
): string => {
  if (message.channel === 'whisper') {
    if (message.sender_character_id === characterId) {
      return `[Whisper -> ${message.target_character_name ?? message.target_character_id ?? 'Unknown'}] ${message.text}`;
    }
    return `[Whisper] ${message.sender_name}: ${message.text}`;
  }
  if (message.channel === 'party') {
    return `[Party] ${message.sender_name}: ${message.text}`;
  }
  return `[Region] ${message.sender_name}: ${message.text}`;
};

export const formatRejectMessage = (
  reasonCode: string,
  message: string,
  chatMessageMaxLength: number,
): string => {
  switch (reasonCode) {
    case 'movement.destination_blocked':
      return 'Movement failed: the destination is blocked by terrain or an obstacle.';
    case 'movement.destination_out_of_bounds':
      return 'Movement failed: the destination is outside the current region bounds.';
    case 'movement.path_unreachable':
      return 'Movement failed: no legal path reaches that destination.';
    case 'movement.path_budget_exceeded':
      return 'Movement failed: pathfinding budget was exceeded before resolving a safe route.';
    case 'movement.geodata_unavailable':
      return 'Movement failed: region geodata is currently unavailable.';
    case 'movement.geodata_mismatch':
      return 'Movement failed: the server rejected the position against the current geodata version.';
    case 'combat.target_out_of_range':
      return 'Skill failed: target is out of range.';
    case 'combat.target_dead':
      return 'Skill failed: target is already dead.';
    case 'combat.actor_dead':
      return 'Action failed: actor is currently dead.';
    case 'world.entity_not_known':
      return 'Skill failed: target is no longer known in the current region.';
    case 'world.entity_not_interactable':
      return 'NPC interaction failed: this entity has no interaction in the current slice.';
    case 'world.loot_out_of_reach':
      return 'Loot failed: item is still out of reach.';
    case 'loot.already_collected':
      return 'Loot failed: item was already collected.';
    case 'loot.party_ineligible':
      return 'Loot failed: item is reserved for another eligible party member.';
    case 'npc.interaction_out_of_range':
      return 'NPC interaction failed: target is no longer within interaction range.';
    case 'npc.action_not_supported':
      return 'NPC interaction failed: that action is not available for this NPC.';
    case 'quest.action_unavailable':
      return 'Quest action failed: the selected quest step is not currently available.';
    case 'inventory.item_not_found':
      return 'Equipment failed: item is no longer in the inventory.';
    case 'inventory.item_not_equippable':
      return 'Equipment failed: item cannot be equipped.';
    case 'inventory.item_not_usable':
      return 'Consumable use failed: item cannot be used right now.';
    case 'inventory.slot_empty':
      return 'Equipment failed: slot is already empty.';
    case 'inventory.slot_mismatch':
      return 'Equipment failed: item does not match the slot.';
    case 'inventory.item_not_stackable':
      return 'Stack action failed: item is not stackable.';
    case 'inventory.split_invalid_quantity':
      return 'Stack split failed: quantity is invalid for that stack.';
    case 'inventory.merge_invalid':
      return 'Stack merge failed: those stacks cannot be merged.';
    case 'pet.target_not_tameable':
      return 'Taming failed: target cannot become a companion in this slice.';
    case 'pet.tame_out_of_range':
      return 'Taming failed: target is outside tame range.';
    case 'pet.ownership_limit_reached':
      return 'Taming failed: this slice allows only one companion per character.';
    case 'pet.not_owned':
      return 'Companion action failed: character does not own that companion.';
    case 'pet.already_summoned':
      return 'Summon failed: companion is already present.';
    case 'pet.not_summoned':
      return 'Companion action failed: companion is not currently summoned.';
    case 'mount.not_mountable':
      return 'Mount failed: companion cannot be mounted in this slice.';
    case 'mount.pet_not_ready':
      return 'Mount failed: summon the companion before mounting.';
    case 'mount.already_mounted':
      return 'Mount failed: character is already mounted.';
    case 'mount.not_mounted':
      return 'Dismount failed: character is not currently mounted.';
    case 'mount.dismount_required':
      return 'Dismiss failed: dismount before dismissing the companion.';
    case 'economy.offer_not_found':
      return 'Vendor purchase failed: offer is not available.';
    case 'economy.vendor_out_of_range':
      return 'Vendor purchase failed: merchant is out of range.';
    case 'economy.insufficient_funds':
      return 'Vendor purchase failed: Duskgold is insufficient.';
    case 'economy.exchange_offer_not_found':
      return 'Exchange failed: offer is not available.';
    case 'economy.exchange_out_of_range':
      return 'Exchange failed: merchant is out of range.';
    case 'economy.exchange_insufficient_materials':
      return 'Exchange failed: required materials are insufficient.';
    case 'economy.storage_out_of_range':
      return 'Warehouse action failed: vaultkeeper is out of range.';
    case 'economy.storage_item_not_found':
      return 'Warehouse action failed: item is no longer available in that container.';
    case 'economy.storage_invalid_quantity':
      return 'Warehouse action failed: quantity is invalid for that item.';
    case 'economy.sell_item_not_found':
      return 'Vendor sale failed: item is no longer in the inventory.';
    case 'economy.sell_invalid_quantity':
      return 'Vendor sale failed: quantity is invalid for that item.';
    case 'economy.sell_not_allowed':
      return 'Vendor sale failed: item cannot be sold here.';
    case 'trade.target_not_known':
      return 'Trade offer failed: player is no longer nearby.';
    case 'trade.target_out_of_range':
      return 'Trade offer failed: player is out of trade range.';
    case 'trade.item_not_found':
      return 'Trade offer failed: item is no longer in the inventory.';
    case 'trade.invalid_quantity':
      return 'Trade offer failed: quantity is invalid for that item.';
    case 'trade.actor_busy':
      return 'Trade offer failed: you already have a pending trade.';
    case 'trade.target_busy':
      return 'Trade offer failed: that player already has a pending trade.';
    case 'trade.offer_not_found':
      return 'Trade action failed: offer is no longer available.';
    case 'trade.offer_not_recipient':
      return 'Trade action failed: this offer belongs to another player.';
    case 'party.target_not_known':
      return 'Party invite failed: player is no longer known in the current region.';
    case 'party.target_not_online':
      return 'Party invite failed: player is no longer available for party invitation.';
    case 'party.target_already_in_party':
      return 'Party invite failed: player is already in a party.';
    case 'party.target_invalid':
      return 'Party invite failed: character cannot invite itself.';
    case 'party.invite_already_pending':
      return 'Party invite failed: player already has a pending party invite.';
    case 'party.invite_not_found':
      return 'Party action failed: invite is no longer available.';
    case 'party.invite_not_recipient':
      return 'Party action failed: this invite belongs to another player.';
    case 'party.invite_expired':
      return 'Party action failed: invite has expired.';
    case 'party.leader_required':
      return 'Party action failed: only the current leader can do that.';
    case 'party.not_in_party':
      return 'Party action failed: character is not currently in a party.';
    case 'party.already_in_party':
      return 'Party action failed: character is already in a party.';
    case 'party.member_not_found':
      return 'Party action failed: player is not currently in the party.';
    case 'party.cannot_kick_self':
      return 'Party action failed: leader cannot remove itself.';
    case 'clan.invalid_name':
      return 'Clan create failed: name is invalid.';
    case 'clan.name_too_short':
      return 'Clan create failed: name is too short.';
    case 'clan.name_too_long':
      return 'Clan create failed: name is too long.';
    case 'clan.name_contains_invalid_characters':
      return 'Clan create failed: name contains invalid characters.';
    case 'clan.name_taken':
      return 'Clan create failed: name is already in use.';
    case 'clan.target_not_known':
      return 'Clan invite failed: player is no longer known in the current region.';
    case 'clan.target_not_online':
      return 'Clan invite failed: player is no longer available for clan invitation.';
    case 'clan.target_already_in_clan':
      return 'Clan invite failed: player is already in a clan.';
    case 'clan.target_invalid':
      return 'Clan invite failed: character cannot invite itself.';
    case 'clan.invite_already_pending':
      return 'Clan invite failed: player already has a pending clan invite.';
    case 'clan.invite_not_found':
      return 'Clan action failed: invite is no longer available.';
    case 'clan.invite_not_recipient':
      return 'Clan action failed: this invite belongs to another player.';
    case 'clan.invite_expired':
      return 'Clan action failed: invite has expired.';
    case 'clan.leader_required':
      return 'Clan action failed: only the current leader can do that.';
    case 'clan.not_in_clan':
      return 'Clan action failed: character is not currently in a clan.';
    case 'clan.already_in_clan':
      return 'Clan action failed: character is already in a clan.';
    case 'clan.member_not_found':
      return 'Clan action failed: player is not currently in the clan.';
    case 'clan.cannot_kick_self':
      return 'Clan action failed: leader cannot remove itself.';
    case 'clan.leader_cannot_leave':
      return 'Clan leave failed: leader must dissolve the clan in this phase.';
    case 'chat.channel_unknown':
      return 'Chat send failed: channel is not available in this slice.';
    case 'chat.region_unavailable':
      return 'Region chat failed: authoritative region is unavailable.';
    case 'chat.message_empty':
      return 'Chat send failed: message cannot be empty.';
    case 'chat.message_too_long':
      return `Chat send failed: message exceeds ${chatMessageMaxLength} characters.`;
    case 'chat.rate_limited':
      return 'Chat send failed: sending too fast. Wait a moment and try again.';
    case 'chat.party_required':
      return 'Party chat failed: character is not currently in a party.';
    case 'chat.whisper_target_required':
      return 'Whisper failed: choose an online target character name.';
    case 'chat.whisper_target_not_found':
      return 'Whisper failed: target player is not online right now.';
    default:
      return `${reasonCode}: ${message}`;
  }
};
