# strong2hevy

`strong2hevy` is a CLI tool for importing workout history from a Strong CSV export into a Hevy account.

It is designed as a review-first migration tool:

- It analyzes your Strong export before importing anything.
- It requires an explicit exercise mapping step because Hevy workout creation needs `exercise_template_id`.
- It can generate Hevy routine candidates from repeated Strong workout names, but routine creation is opt-in.
- It tracks imported workouts locally so reruns do not create duplicates.

## Requirements

- Go 1.25+
- A Strong CSV export
- A Hevy API key

Hevy's public API is documented at <https://api.hevyapp.com/docs/>.

## Build

```bash
go build -o strong2hevy .
```

You can also run it directly with:

```bash
go run .
```

## Input Data

By default, `strong2hevy` reads:

```text
data/strong_workouts.csv
```

You can override that with `--input`.

## Hevy API Key

Pass the API key either as a flag:

```bash
./strong2hevy --api-key <your-key> doctor
```

Or set it in the environment:

```bash
export HEVY_API_KEY=<your-key>
```

## Quick Start

1. Validate your CSV and Hevy connectivity.
2. Generate and review the exercise map.
3. Optionally generate and review routine candidates.
4. Dry-run the workout import.
5. Run the real workout import.

Example:

```bash
./strong2hevy doctor
./strong2hevy exercises resolve
$EDITOR .strong2hevy/exercise-map.yaml
./strong2hevy routines plan
$EDITOR .strong2hevy/routines.yaml
./strong2hevy workouts import --dry-run
./strong2hevy workouts import
```

## Global Flags

These flags work before the command name:

```text
--input <path>      Path to Strong CSV export
--api-key <key>     Hevy API key (or HEVY_API_KEY)
--config <path>     Config file path (default .strong2hevy/config.yaml)
--format <format>   Output format: table or json
```

## Configuration

You can put defaults in `.strong2hevy/config.yaml`.

Example:

```yaml
input: data/strong_workouts.csv
api_key: ""
format: table
state_dir: .strong2hevy
timezone: America/Los_Angeles
weight_unit: lb
distance_unit: mi
default_visibility: private
```

Config fields:

- `input`: Strong CSV path
- `api_key`: Hevy API key. Usually leave this empty and use `HEVY_API_KEY`.
- `format`: `table` or `json`
- `state_dir`: directory for generated state files
- `timezone`: timezone used to interpret Strong timestamps
- `weight_unit`: `lb` or `kg`
- `distance_unit`: `mi`, `km`, or `m`
- `default_visibility`: `private` or `public`

If your Strong export contains distance-based rows and `distance_unit` is not set, `doctor` warns and `workouts import` fails fast.

## Commands

### `doctor`

Validates:

- timezone config
- CSV readability and basic parsing
- API key presence
- Hevy user info access
- Hevy exercise template fetch/caching

Example:

```bash
./strong2hevy doctor
./strong2hevy doctor --refresh
```

`--refresh` forces a fresh fetch of Hevy exercise templates.

### `analyze`

Summarizes the Strong export:

- total rows and workouts
- unique exercise names
- date range
- warmup/work set counts
- timed, distance, and RPE set counts
- top workout names
- top exercise names
- routine candidate stats

Example:

```bash
./strong2hevy analyze
./strong2hevy --format json analyze
```

### `exercises search <query>`

Searches cached Hevy exercise templates locally.

Examples:

```bash
./strong2hevy exercises search "bench press"
./strong2hevy exercises search --refresh "turkish get up"
```

### `exercises resolve`

Builds or updates `.strong2hevy/exercise-map.yaml`.

Resolution rules:

- Exact normalized title match -> auto-resolved to a Hevy template
- Existing map entry -> preserved
- No exact match -> marked `needs_review: true` with suggestions

Example:

```bash
./strong2hevy exercises resolve
./strong2hevy exercises resolve --refresh
```

Generated file example:

```yaml
exercises:
  - strong_name: Bench Press (Barbell)
    action: use-template
    template_id: b459cba5-cd6d-463c-abd6-54f8eafcadcb
    hevy_title: Bench Press (Barbell)

  - strong_name: Static Back Extension
    action: skip
    needs_review: true
    suggestions:
      - template_id: 1234
        title: Back Extension
        type: duration
        primary_muscle_group: lower_back
        score: 0.72
    custom:
      title: Static Back Extension
      exercise_type: ""
      equipment_category: ""
      muscle_group: ""
```

Supported mapping actions:

- `use-template`: use an existing Hevy exercise template
- `create-custom`: create a Hevy custom exercise using the `custom` metadata
- `skip`: skip this Strong exercise during import

Important:

- `create-custom` only works when `custom.title`, `custom.exercise_type`, `custom.equipment_category`, and `custom.muscle_group` are filled in.
- If `needs_review: true` is still present, imports fail rather than guessing.

### `routines plan`

Builds `.strong2hevy/routines.yaml` from repeated Strong workout names.

For each repeated workout name, the tool computes a dominant workout shape and chooses the most recent workout in that dominant shape as the representative routine.

Example:

```bash
./strong2hevy routines plan
```

Generated file example:

```yaml
routines:
  - workout_name: Reload (Squat)
    occurrences: 44
    dominant_occurrences: 14
    stability: 0.318
    suggested: false
    selected: false
    representative:
      source_date: 2020-02-06 16:04:12
      duration: 1h 12m
      exercises:
        - strong_name: Squat (Barbell)
          set_count: 7
          warmup_count: 2
        - strong_name: Overhead Press (Barbell)
          set_count: 5
          warmup_count: 0
```

Review the file and set `selected: true` for routines you want to create.

### `routines apply`

Creates selected routines in Hevy from `.strong2hevy/routines.yaml`.

Examples:

```bash
./strong2hevy routines apply --dry-run
./strong2hevy routines apply
./strong2hevy routines apply --folder "My Programs"
./strong2hevy routines apply --update-existing
```

Flags:

- `--plan <path>`: routine plan file
- `--map <path>`: exercise map file
- `--folder <name|id>`: optional Hevy routine folder
- `--update-existing`: update an existing Hevy routine matched by exact title
- `--dry-run`: build requests without sending them
- `--refresh`: refresh exercise template cache

### `workouts import`

Imports completed Strong workouts into Hevy.

Examples:

```bash
./strong2hevy workouts import --dry-run
./strong2hevy workouts import
./strong2hevy workouts import --from 2024-01-01 --to 2024-12-31
./strong2hevy workouts import --visibility public
./strong2hevy workouts import --timezone America/Los_Angeles --distance-unit mi
```

Flags:

- `--from YYYY-MM-DD`: import workouts on or after this date
- `--to YYYY-MM-DD`: import workouts on or before this date
- `--map <path>`: exercise map file
- `--state <path>`: import state file
- `--visibility private|public`: workout visibility
- `--timezone <iana-name>`: timezone used to parse Strong timestamps
- `--distance-unit mi|km|m`: distance unit used in the Strong CSV
- `--dry-run`: build requests without sending them
- `--refresh`: refresh exercise template cache

Import behavior:

- Strong `Set Order = W` becomes a Hevy `warmup` set.
- Numeric Strong set orders become normal sets.
- Weight is converted to `weight_kg`.
- RPE is passed through when valid for Hevy.
- Workout end time is computed from `Date + Duration`.
- Exercises with `action: skip` are omitted from the workout.
- Imported workouts are tracked in the local import state file.

## Generated Files

By default, `strong2hevy` writes:

```text
.strong2hevy/config.yaml
.strong2hevy/exercise-map.yaml
.strong2hevy/routines.yaml
.strong2hevy/import-state.json
.strong2hevy/exercise-templates-cache.json
```

What they are for:

- `config.yaml`: default local settings
- `exercise-map.yaml`: Strong exercise -> Hevy template/custom decision
- `routines.yaml`: routine candidates and selection state
- `import-state.json`: imported workout hashes and Hevy workout IDs
- `exercise-templates-cache.json`: cached Hevy exercise templates

## Recommended Workflow

### One-time migration

```bash
./strong2hevy doctor
./strong2hevy analyze
./strong2hevy exercises resolve
$EDITOR .strong2hevy/exercise-map.yaml
./strong2hevy workouts import --dry-run
./strong2hevy workouts import
```

### Add routines too

```bash
./strong2hevy routines plan
$EDITOR .strong2hevy/routines.yaml
./strong2hevy routines apply --dry-run
./strong2hevy routines apply
```

## Limitations

- This is not a live sync tool.
- Duplicate prevention is local-state-based, not remote reconciliation against Hevy history.
- There is no delete command.
- Routine planning is heuristic and intentionally requires manual review.
- Custom exercise creation is explicit; the tool does not infer exercise metadata automatically.
- If you change the CSV content after importing, local workout hashes may no longer line up with previous state.

## Troubleshooting

### `missing Hevy API key`

Set `HEVY_API_KEY` or pass `--api-key`.

### `distance rows exist but distance_unit is not configured`

Set `distance_unit` in config or pass `--distance-unit`.

### `exercise "<name>" still needs review in exercise map`

Open `.strong2hevy/exercise-map.yaml` and resolve that entry by choosing:

- `use-template`
- `create-custom`
- `skip`

### Import created nothing

Common causes:

- the workout hashes already exist in `.strong2hevy/import-state.json`
- all exercises in the target workouts are mapped to `skip`
- `--from` / `--to` filtered everything out

## Development

Run tests:

```bash
go test ./...
```
