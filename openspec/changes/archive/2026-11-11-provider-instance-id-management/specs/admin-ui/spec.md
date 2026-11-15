## ADDED Requirements

### Requirement: Provider instance ID editing
The Providers workspace SHALL show each canonical instance ID and provide an explicit, accessible way to edit it. Before confirmation, it SHALL warn that clients and configurations using the old ID must be updated, including stored model selections that cannot safely be rewritten. It SHALL display validation or server errors and show the persisted ID after success without disrupting name or alias editing.

#### Scenario: Successful edit
- **WHEN** an operator enters and confirms a valid available ID
- **THEN** the displayed provider shows the new canonical ID and continues to offer its existing management actions

#### Scenario: Invalid or conflicting ID
- **WHEN** an edit is rejected
- **THEN** the existing ID remains displayed and an actionable error is visible

#### Scenario: Canceled edit
- **WHEN** an operator cancels an ID change before confirming it
- **THEN** the instance ID is not changed
