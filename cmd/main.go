package main

import (
	"log"

	"github.com/IlliaVern/go-todo-app"
	handler "github.com/IlliaVern/go-todo-app/pkg/handlers"
)

func main() {
	handlers := new(handler.Handler)

	srv := new(todo.Server)
	if err := srv.Run("8000", handlers.InitRoutes()); err != nil {
		log.Fatalf("Error ocurred while running http server: %s\n", err.Error())
	}
}
