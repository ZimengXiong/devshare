package main

import (
	"fmt"
	"os"
)

const version = "0.3.8"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		runServer()
	case "publish":
		publish()
	case "serve":
		serve()
	case "list", "ls":
		listShares()
	case "remove", "rm":
		removeShare()
	case "auth":
		auth()
	case "token":
		tokenCommand()
	case "version", "--version":
		fmt.Println("devshare", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`devshare — publish a page or share a local server

  devshare auth login --url https://share.example.com --token ds_...
  devshare publish [--public] [--keep|--ttl 2h] ./dist
  devshare publish --update <share-id-or-url> ./dist
  devshare serve 5173 [--public] [--ttl 2h]
  devshare list
  devshare rm <share-id>
  devshare server
`)
}
