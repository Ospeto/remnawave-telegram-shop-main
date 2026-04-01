## ADDED Requirements

### Requirement: Process-safe container healthcheck
The deployment SHALL use a healthcheck path that does not start a second full application process.

#### Scenario: Container healthcheck executes
- **WHEN** the runtime healthcheck is invoked by the container platform
- **THEN** it performs a lightweight probe against the running service instead of starting the application binary again

### Requirement: Scheduled backup completion bookkeeping
The backup scheduler SHALL record a scheduled run as completed only after local backup generation succeeds.

#### Scenario: Scheduled backup fails before local artifact creation
- **WHEN** a scheduled backup attempt fails before a backup artifact is created
- **THEN** the scheduler does not mark that day as completed and the run remains eligible for retry

#### Scenario: Scheduled backup succeeds
- **WHEN** a scheduled backup creates a local backup artifact successfully
- **THEN** the scheduler marks that day as completed and does not run another scheduled backup for the same day

### Requirement: Unsafe live restore is rejected
The running bot service SHALL reject database restore execution from the live runtime process.

#### Scenario: Admin attempts runtime restore
- **WHEN** an admin requests restore through the running bot service
- **THEN** the service rejects the request with guidance to use an offline/manual restore workflow
