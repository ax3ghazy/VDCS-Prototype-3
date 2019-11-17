package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"text/scanner"
	"time"
)

//test the directory service
//sending tokens was not tested
func main() {
	createEmptyDS()
	id := 0
	var k RegisterationMessage
	k.Server.IP = []byte("192.168.1.1")
	k.Server.Port = 3030
	k.Server.PublicKey = []byte("abcsjsjsa")
	k.Server.NumberOfGates = 321
	k.Server.FeePerGate = 256.55
	k.Type = "Server"
	writeToDS(k, &id)
	writeToDS(k, &id)
	var k2 RegisterationMessage
	k2.Server.IP = []byte("192.168.1.1")
	k2.Server.Port = 3030
	k2.Server.PublicKey = []byte("abcsjsjsa")
	k2.Type = "Client"
	writeToDS(k2, &id)
	writeToDS(k2, &id)
	k2.Server.IP = []byte("192.168.3.1")
	k2.Server.Port = 3030
	k2.Server.PublicKey = []byte("abjsjsa")
	writeToDS(k2, &id)

}

//read Directory Service word by word
func readFromDS() []string {
	file, err := os.Open("DirectoryService.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	scanner.Split(bufio.ScanWords)

	var words []string

	for scanner.Scan() {
		words = append(words, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return words
}

//check if new Server is valid or has already been registered
func validNewServer(lines []string, k ServerInfo) bool {
	for i := 1; i < len(lines); i += 7 {
		if lines[i-1] == "Server" {
			if lines[i] == string(k.IP) {
				return false
			} else if lines[i+2] == string(k.PublicKey) {
				return false
			}
		}
	}
	return true

}

//check if a registered User sends his correct information
func validRegisteredUser(lines []string, k RegisterationMessage, t Token) bool {
	//case Server
	if k.Type == "Server" {
		for i := 1; i < len(lines); i += 7 {
			if lines[i-1] == "Server" && lines[i] == string(k.Server.IP) && lines[i+2] == string(k.Server.PublicKey) && lines[i+5] == string(t.TokenGen) {
				return true
			}

		}
		return false
	} else {
		//case Client
		for i := 1; i < len(lines); i += 7 {
			if lines[i-1] == "Client" && lines[i] == string(k.Server.IP) && lines[i+2] == string(k.Server.PublicKey) && lines[i+5] == string(t.TokenGen) {
				return true
			}

		}
		return false

	}

}

//Write to Directory Service
func writeToDS(k RegisterationMessage, id *int) {
	//write new server to the directory service
	if k.Type == "Server" {
		if validNewServer(readFromDS(), k.Server) {
			token := string(k.Server.IP) + strconv.FormatInt(int64(k.Server.Port), 10) + string(k.Server.PublicKey) + strconv.FormatInt(int64(k.Server.NumberOfGates), 10) + strconv.FormatFloat(k.Server.FeePerGate, 'f', 6, 64)
			token = strconv.FormatInt(int64(*id), 10) + generateToken(token)
			*id++
			f, err := os.OpenFile("DirectoryService.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte("Server " + string(k.Server.IP) + " " + strconv.FormatInt(int64(k.Server.Port), 10) + " " + string(k.Server.PublicKey) + " " + strconv.FormatInt(int64(k.Server.NumberOfGates), 10) + " " + strconv.FormatFloat(k.Server.FeePerGate, 'f', 6, 64) + " " + string(token) + "\n")); err != nil {
				log.Fatal(err)
			}
			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
			var t Token
			t.TokenGen = []byte(token)
			GetFromServer(t, k.Server.IP, k.Server.Port)
			println("New Server been registered")

		} else {
			println("Server Has already been registered")
		}
	} else if k.Type == "Client" { //write new Client to the directory service
		if validNewClient(readFromDS(), k.Server) {
			token := string(k.Server.IP) + strconv.FormatInt(int64(k.Server.Port), 10) + string(k.Server.PublicKey) + strconv.FormatInt(int64(k.Server.NumberOfGates), 10) + strconv.FormatFloat(k.Server.FeePerGate, 'f', 6, 64)
			token = strconv.FormatInt(int64(*id), 10) + generateToken(token)
			*id++
			f, err := os.OpenFile("DirectoryService.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte("Client " + string(k.Server.IP) + " " + strconv.FormatInt(int64(k.Server.Port), 10) + " " + string(k.Server.PublicKey) + " NULL NULL " + string(token) + "\n")); err != nil {
				log.Fatal(err)
			}
			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
			var t Token
			t.TokenGen = []byte(token)
			GetFromClient(t, k.Server.IP, k.Server.Port)
			println("New Client been registered")

		} else {
			println("Client Has already been registered")
		}

	}
}

//check if new Client is valid or has already been registered
func validNewClient(lines []string, k ServerInfo) bool {
	for i := 1; i < len(lines); i += 7 {
		if lines[i-1] == "Client" {
			if lines[i] == string(k.IP) {
				return false
			} else if lines[i+2] == string(k.PublicKey) {
				return false
			}
		}
	}
	return true

}

//break string to array of characters
func breakToCharSlice(str string) []string {

	tokens := []rune(str)

	var result []string

	for _, char := range tokens {
		result = append(result, scanner.TokenString(char))
	}

	return result
}

//shuffle array of characters
func shuffle(src []string) []string {
	final := make([]string, len(src))
	rand.Seed(time.Now().UTC().UnixNano())
	perm := rand.Perm(len(src))

	for i, v := range perm {
		final[v] = src[i]
	}
	return final
}

//generate token based on the server information and shuffling this information
func generateToken(str string) string {
	str = strings.Join(shuffle(breakToCharSlice(str)), "")
	return strings.Replace(str, "\"", "", -1)
}

//create Empty DirectorySevice
func createEmptyDS() {

	var file, err = os.Create("DirectoryService.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fmt.Println("File Created Successfully", "DirectoryService.txt")
}
