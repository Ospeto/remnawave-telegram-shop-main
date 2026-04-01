## ADDED Requirements

### Requirement: Admin-only backup command execution
The system SHALL execute backup and restore commands only for `ADMIN_TELEGRAM_ID` and SHALL reject unauthorized users.

#### Scenario: Unauthorized user attempts backup
- **WHEN** a non-admin Telegram user sends `/backup now`
- **THEN** the system rejects the command and does not start any backup job

#### Scenario: Admin runs backup command
- **WHEN** the admin user sends `/backup now`
- **THEN** the system starts a backup job and reports result back to admin chat

### Requirement: Manual database backup generation
The system SHALL generate a PostgreSQL database dump and compress it into a timestamped `.sql.gz` file on manual backup command.

#### Scenario: Manual backup succeeds
- **WHEN** admin runs `/backup now` and DB is healthy
- **THEN** a new local backup file `db_YYYYMMDD_HHMMSS.sql.gz` is created

### Requirement: Scheduled daily backup
The system SHALL support daily scheduled backup execution with explicit timezone configuration and default Myanmar schedule.

#### Scenario: Daily schedule triggers backup
- **WHEN** backup scheduling is enabled and scheduled time is reached
- **THEN** the system runs a backup job without manual admin input

### Requirement: Telegram delivery for successful backups
The system SHALL send successful backup artifacts to `ADMIN_TELEGRAM_ID` via Telegram document delivery.

#### Scenario: Backup file delivered to admin
- **WHEN** a backup job completes and Telegram upload succeeds
- **THEN** the admin receives the backup document with job metadata

#### Scenario: Telegram upload fails
- **WHEN** backup file generation succeeds but upload fails
- **THEN** the system keeps the local backup file and sends an error status message to admin

### Requirement: Backup retention management
The system SHALL prune old local backups according to configured retention policy (age and/or max file count).

#### Scenario: Retention cleanup removes expired backups
- **WHEN** a backup job completes and local backups exceed retention limits
- **THEN** oldest or expired backups are removed until policy is satisfied

### Requirement: Two-step restore confirmation
The system SHALL require explicit confirmation token approval before running a restore operation.

#### Scenario: Restore requested but not confirmed
- **WHEN** admin sends `/restore latest`
- **THEN** the system returns a short-lived confirmation token and does not restore yet

#### Scenario: Restore confirmed with valid token
- **WHEN** admin sends `/restore confirm <token>` and token is valid
- **THEN** the system begins restore workflow

### Requirement: Pre-restore safety backup
The system SHALL create a fresh safety backup before applying a requested restore.

#### Scenario: Safety backup before restore
- **WHEN** restore confirmation is accepted
- **THEN** the system creates a pre-restore backup before overwriting current DB contents

### Requirement: Backup and restore job safety
The system SHALL enforce single-flight backup/restore execution and SHALL report operation failures to admin.

#### Scenario: Concurrent operation attempt
- **WHEN** a backup or restore is already running and another backup/restore command is received
- **THEN** the new request is rejected with a busy status message

#### Scenario: Job failure notification
- **WHEN** backup or restore fails at any stage
- **THEN** the system sends an explicit failure message to admin and logs structured error details
