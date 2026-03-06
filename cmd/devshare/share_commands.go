package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func listShares() {
	c := client()
	req, _ := http.NewRequest("GET", c.URL+"/v1/shares", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatal(resp.Status)
	}
	var rows []shareResponse
	_ = json.NewDecoder(resp.Body).Decode(&rows)
	for _, share := range rows {
		fmt.Printf("%-18s %-8s %-8s %s\n", share.ID, share.Kind, share.Visibility, share.URL)
	}
}

func removeShare() {
	if len(os.Args) != 3 {
		log.Fatal("usage: devshare rm <share-id>")
	}
	c := client()
	req, _ := http.NewRequest("DELETE", c.URL+"/v1/shares/"+os.Args[2], nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatal(resp.Status)
	}
	fmt.Println("removed", os.Args[2])
}
