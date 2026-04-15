package main

import (
	"log"
	"net/http"
	"os"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

func main() {
	if err := loadEnvFile(".env"); err != nil && !os.IsNotExist(err) {
		log.Printf("skip .env loading: %v", err)
	}

	app := pocketbase.New()
	pushService, err := newPushService(app)
	if err != nil {
		log.Fatal(err)
	}

	app.Logger().Info("push notifications enabled", "projectId", pushService.client.ProjectID())

	jsvm.MustRegister(app, jsvm.Config{
		OnInit: func(vm *goja.Runtime) {
			registerPushBindings(vm, pushService)
		},
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if !se.Router.HasRoute(http.MethodGet, "/{path...}") {
			se.Router.GET("/{path...}", apis.Static(os.DirFS("./pb_public"), false))
		}
		return se.Next()
	})

	app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
		pushService.Shutdown()
		return te.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
