# Security Review Checklist

## Identity and Access

- Who is the actor?
- What can the actor read, write, or trigger?
- What prevents account hijacking, character hijacking, or unauthorized action submission?
- Which framework or provider boundaries could accidentally bypass internal authorization or validation?

## Integrity

- Which actions can the client request but never decide?
- Can the client attempt to smuggle authoritative path, waypoint, collision, geodata, price, damage, or identity data?
- Which events need immutable audit records?
- What protects against replay or duplicate submission?

## Data Protection

- Which data is sensitive or regulated?
- Where is it stored, logged, or transmitted?
- Which secrets or tokens require rotation and least privilege?
- Which third-party webhooks must be signature-verified before any data is trusted?

## Abuse Resistance

- What stops automated spam or brute-force behavior?
- What stops impossible movement, obstacle bypassing, or repeated blocked-destination probing?
- What prevents economic exploits or impossible state transitions?
- What evidence remains for post-incident investigation?
