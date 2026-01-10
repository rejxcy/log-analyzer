# Known Issues Rules

This directory contains YAML rule files for identifying known error patterns.

## Rule File Format

Each rule file should follow this structure:

```yaml
- id: UNIQUE-ID
  name: "Human readable name"
  category: "error_category"
  severity: low|medium|high|critical
  content_patterns:
    - "regex pattern 1"
    - "regex pattern 2"
  caller_patterns:  # optional
    - "file pattern"
  services:
    - "service-name-1"
    - "service-name-2"
  description: |
    Detailed description of the issue
  suggested_actions:
    - "Action 1"
    - "Action 2"
  alert_threshold:  # optional
    total: 100
    or_density: 50
```

## Example Files

- `player-errors.yaml` - Player-related errors
- `database-errors.yaml` - Database connection and query errors
- `api-errors.yaml` - API-related errors
- `network-errors.yaml` - Network connectivity errors