package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"pbmanager/backend/internal/manager"
)

func main() {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: os.Getenv("PBM_DATA_DIR")})
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-admin" {
		var credentials struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(os.Stdin).Decode(&credentials); err != nil {
			log.Fatal(err)
		}
		if credentials.Email == "" || len(credentials.Password) < 20 {
			log.Fatal("email and a password of at least 20 characters are required")
		}
		if err := app.Bootstrap(); err != nil {
			log.Fatal(err)
		}
		defer app.ResetBootstrapState()
		if _, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, credentials.Email); err == nil {
			log.Fatal("administrator already exists")
		}
		collection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err != nil {
			log.Fatal(err)
		}
		record := core.NewRecord(collection)
		record.SetEmail(credentials.Email)
		record.SetPassword(credentials.Password)
		if err = app.Save(record); err != nil {
			log.Fatal(err)
		}
		return
	}

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{Automigrate: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.OnServe().BindFunc(func(event *core.ServeEvent) error {
		control, err := manager.FromEnvironment(app)
		if err != nil {
			return err
		}
		control.Register(event.Router)
		if dir := os.Getenv("PBM_FRONTEND_DIR"); dir != "" {
			event.Router.GET("/{path...}", apis.Static(os.DirFS(dir), true))
		}
		control.Start(ctx)
		return event.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
