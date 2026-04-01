## ADDED Requirements

### Requirement: Graceful termination on container stop signals
The running application SHALL execute its graceful shutdown path when the process receives standard container termination signals.

#### Scenario: Process receives SIGTERM
- **WHEN** the application receives `SIGTERM`
- **THEN** the main context is canceled and the HTTP server shutdown path is executed

#### Scenario: Process receives interrupt signal
- **WHEN** the application receives `os.Interrupt`
- **THEN** the application follows the same graceful shutdown path used for normal termination
