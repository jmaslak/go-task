# task

A to-do list manager that keeps each task in its own plain text file.

This is a Go port of [App::Tasks](https://github.com/jmaslak/Raku-app-task),
the Raku application of the same name. It reads and writes the same task
files, so an existing `~/.task` directory carries over untouched. The port
was made utilizing a LLM for machine translation between languages.

## Installing

```sh
go install github.com/jmaslak/go-task/cmd/task@latest
```

Or build from a checkout:

```sh
go build -o task ./cmd/task
```

Go 1.27 or newer is required. The `less` pager and the `nano` editor are used
by default; both are configurable.

## Synopsis

```sh
task new                # Add a new task
task list               # List existing tasks
task <num>              # Show information about a task
task note <num>         # Add notes to a task
task close <num>        # Close a task
```

Run `task` with no arguments for a menu of every command, and `task help
<command>` for the details of one.

## Goals

- Task data stays in plain text
- Each task is one file
- Simple things are simple
- Scriptable from a shell
- Works over a plain Unix shell account
- Pending work on a task is tracked as notes

## Environment

### `TASKDIR`

Where the task files live. Open tasks sit in this directory, and closed ones
are moved into `done/` underneath it.

The default is `$HOME/.task`, or `.task` under the working directory when
`HOME` is not set.

## Commands

### `new`

```sh
task new
task new <title>
task --expire-today new <title>
task --maturity-date=2099-12-31 new <title>
task --tag=foo new <title>
```

Creates a task. Given a title on the command line, the task is created with an
empty body. Without one, the title is asked for, and then an optional note in
your editor.

`--expire-today` gives the new task an expiration date of today; see
[`expire`](#expire). `--maturity-date` sets its maturity date; see
[`set-maturity`](#set-maturity). The two cannot be combined. `--tag` tags the
new task; see [`add-tag`](#add-tag).

### `list`

```sh
task list
task list <max-items>
task --show-immature list
task --all list
task --tag=foo list
```

Displays the open tasks. Tasks that have not reached their maturity date are
left out unless `--show-immature` or `--all` is given. `--all` additionally
shows tasks held back for the day by a display frequency, and tasks carrying a
tag from the `ignore-tags` section of the configuration file.

`--tag` narrows the listing to tasks carrying that tag, including tasks that
would otherwise be hidden by `ignore-tags`.

An integer argument caps how many tasks are listed.

### `show`

```sh
task <task-number>
task show <task-number>
```

Displays a task's headers and every note on it, through the pager.

### `monitor`

```sh
task monitor
task --show-immature monitor
task --all monitor
```

Displays a task list that refreshes once a second and fills the screen. Any
key exits; Ctrl-L redraws. `--show-immature`, `--all`, and `--tag` work as
they do for `list`.

### `note`

```sh
task note <task-number>
task note <task-number> <note>
```

Appends a note to a task. Without the note as an argument, your editor is
opened to write one. Notes are shown by [`show`](#show).

### `close`

```sh
task close <task-number>
```

Closes a task, moving its file into `done/`. You are offered the chance to add
a closing note first. This coalesces afterwards, so task numbers change.

### `retitle`

```sh
task retitle <task-number> [title]
```

Changes a task's title. The old title is kept as a note.

### `move`

```sh
task move <task-number> <new-number>
```

Moves a task to a different position in the list, shifting the tasks in
between out of its way.

### `set-expire`

```sh
task set-expire <task-number> [YYYY-MM-DD]
```

Sets the last day on which the task is worth doing. Buying a Christmas turkey
stops being a useful task on December 26th, so such a task might expire on the
25th. Expired tasks are closed by [`expire`](#expire).

### `expire`

```sh
task expire
```

Closes every open task whose expiration date has passed. Suitable for a daily
crontab entry. This coalesces afterwards, so task numbers change.

### `set-frequency`

```sh
task set-frequency <task-number> [days]
```

Displays the task on only one day out of every `N`. A frequency of `7` shows
the task about once a week. Which day of the cycle a task falls on is derived
from its task ID, so tasks sharing a frequency do not all come due together.

The point is to keep a long tail of low priority tasks from swamping the
listing.

### `set-maturity`

```sh
task set-maturity <task-number> [YYYY-MM-DD]
```

Sets the first day the task is displayed. Before that day it is left out of
`list` and `monitor` unless `--show-immature` or `--all` is given.

### `add-tag` / `remove-tag`

```sh
task add-tag <task-number> <tag>
task remove-tag <task-number> <tag>
```

Adds or removes a tag, which is any string without whitespace. Tags filter
listings and are displayed alongside task titles.

### `coalesce`

```sh
task coalesce
```

Renumbers the tasks so the first is 1 and there are no gaps. Needed when task
files are removed behind the application's back.

### `trello-sync`

```sh
task trello-sync
```

With a `trello` section in the configuration file, mirrors Trello cards into
the task list. The sync is one way: cards become tasks that cannot be edited
locally, and a task whose card has been moved or deleted is removed on the
next sync.

## Task numbers and stale listings

The commands that act on a task by number refuse to run if the task list has
changed since it was last displayed in this terminal, because the number you
are typing may no longer mean the task you meant. Run `task list` first.

The check applies only when the input is a terminal, so scripts and cron jobs
are unaffected.

## Running more than one copy at once

Changes to the task directory are made under a lock, so two copies of `task`
cannot write over each other. The lock is held only for the reads and writes
themselves: prompts, the editor, the pager, and Trello all run with the
directory unlocked, so one copy sitting in an editor does not hold up another.

A command that cannot have the directory within a couple of seconds says so and
exits rather than waiting indefinitely. `monitor` never waits: a refresh that
cannot read the directory keeps the listing it has, notes that it may be out of
date, and picks the changes up on a later second. A keystroke still exits it
whatever the other copies of `task` are doing.

## Using the task directory from other programs

The `task`, `trello` and `config` packages are importable, so other programs
can change the task directory the same way `task` does, under the same lock.
`Store.ArchiveByID` closes a task found by its ID rather than its number,
which suits a program that showed the task some time ago, and renumbers the
tasks left behind. `trello.Client.CloseCard` marks a card's due date
complete and archives it.

## Configuration

Configuration lives in `~/.task.yaml`, with a companion `~/.task.secret.yaml`
for credentials. Both are optional; anything the secret file sets wins.

```yaml
theme: dark
immature-task-color: 'bold red'
editor-command: 'nano +3,1 %FILENAME%'
ignore-tags:
  - someday
monitor:
  display-time: true
trello:
  api-key: abcdef0123456789
  token: 9876543210fedcba
  tasks:
    Board1:
      List1: tag1
```

### `theme`

`dark` (the default), `light`, or `no-color`, which strips the escape codes
out of every message. Themes set all the colors below; naming a color
individually overrides the theme's choice for it.

### Colors

`body-color`, `header-alert-color`, `header-normal-color`,
`header-seperator-color`, `header-title-color`, `immature-task-color`,
`not-displayed-today-color`, `prompt-bold-color`, `prompt-color`,
`prompt-info-color`, `tag-color`, and `reset`.

A color is a space-separated list of attributes: `bold`, `dark`, `italic`,
`underline`, `blink`, `reverse`, `concealed`, the eight named colors and their
`bright_` variants, any of those prefixed with `on_` for a background, or a
number from 0 to 255 selecting from the 256 color palette. The syntax is the
same one the Raku version accepted.

### `ignore-tags`

Tags whose tasks stay out of the default listing. They still show up under
`--all`, or when asked for by `--tag`.

### `editor-command`

The command used to write notes. `%FILENAME%` is replaced with the path of a
scratch file. The default is:

```
nano -b -r 72 -s ispell +3,1 %FILENAME%
```

### `pager-command`

The command used for output that may not fit on a screen. `%FILENAME%` is
replaced with the path of the text to show, and `%PROMPT%` with a prompt for
the reader. The default is:

```
less -RFX -P%PROMPT% -- %FILENAME%
```

An empty `pager-command` writes the output straight to standard output.

### `monitor.display-time`

Whether `monitor` shows a clock above the task list. Defaults to true.

### `trello`

`api-key` and `token` come from a Trello power-up you create yourself.
`tasks` maps a board name to the lists on it worth syncing, and each list to
the tag its tasks are given. `base-url` overrides the API endpoint and exists
for testing.

Credentials belong in `~/.task.secret.yaml` rather than `~/.task.yaml`.

## File format

Every open task is one file named `NNNNN-none.task`, where `NNNNN` is the
zero-padded task number. Closed tasks move to `done/` under a name carrying
the time they were closed, the number they had, and the process that closed
them.

A task file is a set of headers, then any number of notes:

```
Title: Buy a turkey
Created: 1437509667
Task-Id: 6159523072535192340592
Expires: 2026-12-25
Tags: shopping
--- 1538856865
The good butcher closes at noon.
```

`Title`, `Created`, and `Task-Id` are always written. `Expires`, `Tags`,
`Not-Before`, `Display-Frequency`, and `Trello-ID` appear only when set. Each
note is introduced by `--- ` and the Unix time it was written.

Files written by the Raku version had no `Task-Id`; they are given one the
first time they are read, and rewritten.

## Differences from the Raku version

The behavior and the file format are the same. A few things did change:

- **Exit codes.** A command that refuses to run — a stale task list, a task
  number that does not exist, a date in the past — now exits non-zero. The
  Raku version printed the complaint and exited successfully, which a script
  could not tell apart from success.
- **Terminal size.** The size is asked of the terminal driver rather than
  discovered by moving the cursor and parsing the reply, and it is checked on
  every redraw, so `monitor` follows a window that is resized.
- **Writes are atomic.** A task file is written to a temporary file and
  renamed into place, so an interrupted write cannot leave a half written
  task.
- **Terminal identity.** The freshness check identifies a terminal by its
  device number rather than by name. The first run after the switch will ask
  for a fresh `task list`.
- **Blank lines** at the top of a note written in the editor are now stripped,
  which the Raku version intended but did not do.

## Author

Joelle Maslak `jmaslak@antelope.net`

## Legal

Copyright © 2015-2026 by Joelle Maslak

Licensed under the Artistic License 2.0.
