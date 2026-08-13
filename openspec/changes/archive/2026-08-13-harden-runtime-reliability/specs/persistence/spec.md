## MODIFIED Requirements

### Requirement: Request metering
Completed provider calls SHALL record request/model/provider/client/API shape, tokens, latency, streaming, status, errors, and time through a bounded asynchronous writer whose enqueue path remains safe during concurrent worker shutdown or restart.

#### Scenario: Metering buffer full
- **WHEN** the asynchronous metering queue is full
- **THEN** the record is dropped with a warning and the user request is unaffected

#### Scenario: Metering enqueue races with shutdown
- **WHEN** a metering enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock

### Requirement: Access-token usage stamping
Successful access-token use SHALL enqueue last-used timestamp updates without blocking, MAY silently drop an update when its bounded queue is full, and SHALL remain safe during concurrent worker shutdown or restart.

#### Scenario: Full stamp queue
- **WHEN** no queue capacity remains
- **THEN** authentication still succeeds without waiting

#### Scenario: Stamp enqueue races with shutdown
- **WHEN** a token-stamp enqueue occurs concurrently with worker shutdown or restart
- **THEN** the operation completes without panic, data race, or shutdown deadlock

### Requirement: Background worker lifecycle
Database initialization SHALL start metering and token-stamp workers, and shutdown or restart SHALL prevent new enqueue operations from targeting closing queues before draining accepted work.

#### Scenario: Worker restart
- **WHEN** worker startup is requested while workers run
- **THEN** previous workers reject or accept concurrent enqueues safely, stop, and drain accepted work before replacements begin

## ADDED Requirements

### Requirement: Complete database collection reads
Collection-reading persistence operations SHALL return a database iteration or scan error instead of reporting a partial collection as a successful complete result.

#### Scenario: Row iteration fails after valid rows
- **WHEN** a database collection query yields one or more valid rows and then reports an iteration error
- **THEN** the persistence operation returns an error and does not report the partial collection as success

### Requirement: Complete cache snapshot publication
Routing and model-state caches SHALL mark a snapshot loaded and replace the prior published snapshot only after every query, row scan, and iteration for that snapshot completes successfully.

#### Scenario: Virtual-model snapshot load fails
- **WHEN** virtual-model or upstream loading fails before the complete snapshot is available
- **THEN** no partial virtual-model snapshot is published and a later access retries loading

#### Scenario: Provider-instance snapshot load fails
- **WHEN** provider-instance loading fails before the complete snapshot is available
- **THEN** no partial provider-instance snapshot is published and a later access retries loading

#### Scenario: Model-state snapshot load fails
- **WHEN** model-state loading fails before the complete snapshot is available
- **THEN** no partial model-state snapshot is published and a later access retries loading
