// Command task is a to-do list manager that keeps each task in its own plain
// text file.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jmaslak/go-task/config"
	"github.com/jmaslak/go-task/internal/app"
	"github.com/jmaslak/go-task/task"
)

// version is the release this build came from.
const version = "0.5.0"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

// exitCode reports the status to exit with, having printed the error unless
// the user has already been told about it.
func exitCode(err error) int {
	reportError(err)
	if errors.Is(err, app.ErrAborted) {
		// The user backed out; that is not a failure.
		return 0
	}
	return 1
}

// reportError prints an error, unless it is one the user has already been told
// about in the terms they care about.
func reportError(err error) {
	switch {
	case errors.Is(err, app.ErrAborted):
	case errors.Is(err, app.ErrStaleTaskList):
	case errors.Is(err, task.ErrNotFound):
	default:
		fmt.Fprintln(os.Stderr, "task:", err)
	}
}

// cli holds the state shared by the commands.
type cli struct {
	app *app.App

	expireToday  bool
	showImmature bool
	all          bool
	maturityDate string
	tag          string
}

// listOptions returns the listing options the global flags ask for.
func (c *cli) listOptions() app.ListOptions {
	return app.ListOptions{
		ShowImmature: c.showImmature || c.all,
		All:          c.all,
		Tag:          c.tag,
	}
}

// newOptions returns the options a new task should be created with.
func (c *cli) newOptions() (app.NewOptions, error) {
	opts := app.NewOptions{ExpireToday: c.expireToday, Tag: c.tag}
	if c.maturityDate != "" {
		day, err := task.ParseDate(c.maturityDate)
		if err != nil {
			return opts, err
		}
		opts.MaturityDate = day
	}

	return opts, nil
}

func newRootCommand() *cobra.Command {
	c := &cli{}

	root := &cobra.Command{
		Use:   "task [command]",
		Short: "Manage a to-do list kept as plain text files",
		Long: "Manage a to-do list kept as plain text files, one per task.\n\n" +
			"Running task with a task number shows that task. Running it with no\n" +
			"arguments at all offers a menu of the commands below.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configFile, secretFile := config.DefaultPaths()
			cfg, err := config.Load(configFile, secretFile)
			if err != nil {
				return err
			}

			c.app = app.New(task.NewStore(task.DefaultDir()), cfg)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// A bare task number shows that task.
			if len(args) == 1 {
				if number, err := strconv.Atoi(args[0]); err == nil {
					return c.app.Show(number)
				}
			}
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q", args[0])
			}

			return c.menuLoop()
		},
	}

	flags := root.PersistentFlags()
	flags.BoolVar(&c.expireToday, "expire-today", false, "create the task with an expiration date of today")
	flags.BoolVar(&c.showImmature, "show-immature", false, "include tasks that have not reached their maturity date")
	flags.BoolVar(&c.all, "all", false, "include immature tasks, ignored tags, and tasks held back by a display frequency")
	flags.StringVar(&c.maturityDate, "maturity-date", "", "create the task with a maturity date (YYYY-MM-DD)")
	flags.StringVar(&c.tag, "tag", "", "tag to create the task with, or to limit a listing to")

	root.AddCommand(c.newCommand())
	root.AddCommand(c.listCommand())
	root.AddCommand(c.showCommand())
	root.AddCommand(c.monitorCommand())
	root.AddCommand(c.noteCommand())
	root.AddCommand(c.closeCommand())
	root.AddCommand(c.moveCommand())
	root.AddCommand(c.retitleCommand())
	root.AddCommand(c.coalesceCommand())
	root.AddCommand(c.expireCommand())
	root.AddCommand(c.setExpireCommand())
	root.AddCommand(c.setMaturityCommand())
	root.AddCommand(c.setFrequencyCommand())
	root.AddCommand(c.addTagCommand())
	root.AddCommand(c.removeTagCommand())
	root.AddCommand(c.trelloSyncCommand())

	return root
}

func (c *cli) newCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "new [title]",
		Aliases: []string{"add"},
		Short:   "Create a new task",
		Long: "Create a new task. Without a title, the title and an optional note are\n" +
			"asked for interactively.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := c.newOptions()
			if err != nil {
				return err
			}

			_, err = c.app.NewTask(firstArg(args), opts)
			return err
		},
	}
}

func (c *cli) listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list [max-items]",
		Short: "List the open tasks",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := c.listOptions()
			if len(args) == 1 {
				maxval, err := strconv.Atoi(args[0])
				if err != nil || maxval < 1 {
					return fmt.Errorf("invalid maximum number of items: %s", args[0])
				}
				opts.Max = maxval
			}

			return c.app.List(opts)
		},
	}
}

func (c *cli) showCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "show <task-number>",
		Aliases: []string{"view"},
		Short:   "Show a task and its notes",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := c.taskNumber(args, "show")
			if err != nil {
				return err
			}

			return c.app.Show(number)
		},
	}
}

func (c *cli) monitorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "monitor",
		Short: "Display a task list that refreshes until a key is pressed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.app.Monitor(c.listOptions())
		},
	}
}

func (c *cli) noteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "note <task-number> [note]",
		Short: "Add a note to a task",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := c.taskNumber(args, "modify")
			if err != nil {
				return err
			}

			note := ""
			if len(args) > 1 {
				note = args[1]
			}

			return c.app.AddNote(number, note)
		},
	}
}

func (c *cli) closeCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "close <task-number>",
		Aliases: []string{"commit"},
		Short:   "Close a task",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := c.taskNumber(args, "close")
			if err != nil {
				return err
			}

			return c.app.CloseTask(number)
		},
	}
}

func (c *cli) moveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "move <task-number> <new-number>",
		Short: "Move a task to a different position in the list",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := c.taskNumber(args, "move")
			if err != nil {
				return err
			}

			var to int
			if len(args) > 1 {
				if to, err = strconv.Atoi(args[1]); err != nil {
					return fmt.Errorf("invalid task number: %s", args[1])
				}
			} else if to, err = c.app.AskNumber("[task] Please enter desired location of task > "); err != nil {
				return err
			}

			return c.app.Move(from, to)
		},
	}
}

func (c *cli) retitleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "retitle <task-number> [title]",
		Short: "Change the title of a task",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := optionalNumber(args)
			if err != nil {
				return err
			}

			title := ""
			if len(args) > 1 {
				title = args[1]
			}

			return c.app.Retitle(number, title)
		},
	}
}

func (c *cli) coalesceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "coalesce",
		Short: "Renumber the tasks so there are no gaps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.app.Coalesce()
		},
	}
}

func (c *cli) expireCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "expire",
		Short: "Close every task whose expiration date has passed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.app.Expire()
		},
	}
}

func (c *cli) setExpireCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-expire <task-number> [YYYY-MM-DD]",
		Short: "Set the last day a task is relevant",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := optionalNumber(args)
			if err != nil {
				return err
			}

			day, err := optionalDate(args)
			if err != nil {
				return err
			}

			return c.app.SetExpiration(number, day)
		},
	}
}

func (c *cli) setMaturityCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-maturity <task-number> [YYYY-MM-DD]",
		Short: "Set the first day a task is displayed",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := optionalNumber(args)
			if err != nil {
				return err
			}

			day, err := optionalDate(args)
			if err != nil {
				return err
			}

			return c.app.SetMaturity(number, day)
		},
	}
}

func (c *cli) setFrequencyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-frequency <task-number> [days]",
		Short: "Display a task on only one day out of every so many",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := optionalNumber(args)
			if err != nil {
				return err
			}

			var frequency int64
			if len(args) > 1 {
				if frequency, err = strconv.ParseInt(args[1], 10, 64); err != nil || frequency < 1 {
					return errors.New("display frequency must be an integer of at least 1")
				}
			}

			return c.app.SetFrequency(number, frequency)
		},
	}
}

func (c *cli) addTagCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add-tag <task-number> <tag>",
		Short: "Add a tag to a task",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, tag, err := numberAndTag(args)
			if err != nil {
				return err
			}

			return c.app.AddTag(number, tag)
		},
	}
}

func (c *cli) removeTagCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-tag <task-number> <tag>",
		Short: "Remove a tag from a task",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, tag, err := numberAndTag(args)
			if err != nil {
				return err
			}

			return c.app.RemoveTag(number, tag)
		},
	}
}

func (c *cli) trelloSyncCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "trello-sync",
		Short: "Mirror the configured Trello lists into the task list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.app.TrelloSync(cmd.Context())
		},
	}
}

// menuLoop offers the command menu and runs the command chosen, over and over
// until the user quits to the shell. A command that fails is reported and the
// menu comes back, so that one mistake does not end the session.
func (c *cli) menuLoop() error {
	for {
		choice, err := c.app.Menu()
		if err != nil {
			// Nothing left to read: there is no one to show a menu to.
			return err
		}
		if choice == "quit" {
			return nil
		}

		if err := c.runMenuChoice(choice); err != nil {
			reportError(err)
		}
	}
}

// runMenuChoice runs the command the menu returned.
func (c *cli) runMenuChoice(choice string) error {
	switch choice {
	case "new":
		opts, err := c.newOptions()
		if err != nil {
			return err
		}
		_, err = c.app.NewTask("", opts)
		return err
	case "note":
		number, err := c.app.AskTaskNumber("modify")
		if err != nil {
			return err
		}
		return c.app.AddNote(number, "")
	case "show":
		number, err := c.app.AskTaskNumber("show")
		if err != nil {
			return err
		}
		return c.app.Show(number)
	case "close":
		number, err := c.app.AskTaskNumber("close")
		if err != nil {
			return err
		}
		return c.app.CloseTask(number)
	case "move":
		from, err := c.app.AskTaskNumber("move")
		if err != nil {
			return err
		}
		to, err := c.app.AskNumber("[task] Please enter desired location of task > ")
		if err != nil {
			return err
		}
		return c.app.Move(from, to)
	case "list":
		return c.app.List(c.listOptions())
	case "monitor":
		return c.app.Monitor(c.listOptions())
	case "coalesce":
		return c.app.Coalesce()
	case "expire":
		return c.app.Expire()
	case "retitle":
		return c.app.Retitle(0, "")
	case "set-expire":
		return c.app.SetExpiration(0, task.Date{})
	case "set-maturity":
		return c.app.SetMaturity(0, task.Date{})
	case "set-frequency":
		return c.app.SetFrequency(0, 0)
	case "add-tag":
		return c.app.AddTag(0, "")
	case "remove-tag":
		return c.app.RemoveTag(0, "")
	default:
		return fmt.Errorf("unknown command %q", choice)
	}
}

// taskNumber returns the task number a command was given, asking for one when
// it was left out.
func (c *cli) taskNumber(args []string, purpose string) (int, error) {
	if len(args) == 0 {
		return c.app.AskTaskNumber(purpose)
	}

	number, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, fmt.Errorf("invalid task number: %s", args[0])
	}
	return number, nil
}

// optionalNumber returns the task number a command was given, or zero when it
// was left out for the command itself to ask about.
func optionalNumber(args []string) (int, error) {
	if len(args) == 0 {
		return 0, nil
	}

	number, err := strconv.Atoi(args[0])
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid task number: %s", args[0])
	}
	return number, nil
}

// optionalDate parses the date argument a command was given, if any.
func optionalDate(args []string) (task.Date, error) {
	if len(args) < 2 {
		return task.Date{}, nil
	}
	return task.ParseDate(args[1])
}

// numberAndTag parses the arguments the tag commands share.
func numberAndTag(args []string) (int, string, error) {
	number, err := optionalNumber(args)
	if err != nil {
		return 0, "", err
	}

	tag := ""
	if len(args) > 1 {
		tag = args[1]
		if len(strings.Fields(tag)) != 1 {
			return 0, "", errors.New("a tag may not be empty or contain whitespace")
		}
	}

	return number, tag, nil
}

// firstArg returns the first argument, or the empty string when there is
// none.
func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
