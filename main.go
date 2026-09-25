package main

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	debugPath := os.Getenv("TG_DEBUG_LOG")
	if debugPath != "" {
		logFile, err := os.OpenFile(debugPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to open debug log file")
		}

		log.Logger = log.Output(logFile)
		log.Info().Msgf("Enabling debug logging to %s", debugPath)
	}

	app := &cli.Command{
		Name:  "telegram-mcp",
		Usage: "Telegram MCP server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "app-id",
				Usage:    "Telegram App ID",
				Required: true,
				Sources:  cli.EnvVars("TG_APP_ID"),
			},
			&cli.StringFlag{
				Name:     "api-hash",
				Usage:    "Telegram API Hash",
				Required: true,
				Sources:  cli.EnvVars("TG_API_HASH"),
			},
			&cli.StringFlag{
				// gpcv: no default, so this build never picks up a personal
				// ~/.telegram-mcp session by accident.
				Name:     "session",
				Usage:    "Path to session file",
				Required: true,
				Sources:  cli.EnvVars("TG_SESSION_PATH"),
			},
			&cli.StringFlag{
				Name:    "schema-version",
				Usage:   "JSON Schema version URL (e.g. https://json-schema.org/draft-07/schema#)",
				Sources: cli.EnvVars("TG_SCHEMA_VERSION"),
			},
			&cli.BoolFlag{
				Name:        "dry",
				Usage:       "Check the session with GetMe only and print it as JSON",
				Local:       true,
				HideDefault: true,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "auth",
				Usage: "Authenticate with Telegram",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "phone",
						Usage:    "Phone number to authenticate with",
						Required: true,
						Aliases:  []string{"p"},
					},
					&cli.StringFlag{
						Name:        "password",
						Usage:       "Password for 2FA if exists (prefer TG_2FA_PASSWORD env over argv)",
						HideDefault: true,
						Sources:     cli.EnvVars("TG_2FA_PASSWORD"),
					},
					&cli.BoolFlag{
						Name:        "new",
						Usage:       "Remove old session and create new one",
						HideDefault: true,
					},
				},
				Action: authCommand,
			},
		},
		Action: serve,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Msg(err.Error())
	}
}
