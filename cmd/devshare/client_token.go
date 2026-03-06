package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func tokenCommand() {
	if len(os.Args) < 3 || os.Args[2] != "create" {
		log.Fatal("usage: devshare token create --label NAME --scopes publish,list,delete")
	}
	fs := flag.NewFlagSet("token create", flag.ExitOnError)
	label := fs.String("label", "", "token label")
	scopes := fs.String("scopes", "publish,list,delete", "comma-separated scopes")
	_ = fs.Parse(os.Args[3:])
	if strings.TrimSpace(*label) == "" {
		log.Fatal("--label is required")
	}
	c := client()
	body, _ := json.Marshal(tokenInput{Label: *label, Scopes: strings.Split(*scopes, ",")})
	req, _ := http.NewRequest("POST", c.URL+"/v1/tokens", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("server: %s", b)
	}
	var out tokenResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	fmt.Println(out.Token)
	fmt.Println("Save this token now; it will not be shown again.")
}
