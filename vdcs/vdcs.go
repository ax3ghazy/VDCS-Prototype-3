package vdcs

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
)

//Wire wire abstraction
type Wire struct {
	WireID    string `json:"WireID"`
	WireLabel []byte `json:"WireLabel"`
}

//Gate gate abstraction
type Gate struct {
	GateID     string   `json:"GateID"`
	GateInputs []string `json:"GateInputs"`
}

//CircuitGate a gate in a boolean circuit
type CircuitGate struct {
	Gate
	TruthTable []bool `json:"TruthTable"`
}

//GarbledGate a gate in a garbled circuit
type GarbledGate struct {
	Gate
	GarbledValues [][]byte `json:"GarbledValues"`
}

//ComID computation ID abstraction
type ComID struct {
	CID string `json:"ComID"`
}

//Circuit circuit abstraction
type Circuit struct {
	InputGates  []CircuitGate `json:"InputGates"`
	MiddleGates []CircuitGate `json:"MiddleGates"`
	OutputGates []CircuitGate `json:"OutputGates"`
}

//Randomness container for randomness
type Randomness struct {
	Rin       int64 `json:"Rin"`
	Rout      int64 `json:"Rout"`
	Rgc       int64 `json:"Rgc"`
	LblLength int   `json:"LblLength"`
}

//CircuitMessage a complete circuit message
type CircuitMessage struct {
	Circuit
	ComID
	Randomness
}

//GarbledCircuit garbled circuit abstraction
type GarbledCircuit struct {
	InputGates  []GarbledGate `json:"InputGates"`
	MiddleGates []GarbledGate `json:"MiddleGates"`
	OutputGates []GarbledGate `json:"OutputGates"`
	ComID
}

//GarbledMessage complete garbled circuit message
type GarbledMessage struct {
	InputWires []Wire `json:"InputWires"`
	GarbledCircuit
	OutputWires []Wire `json:"OutputWires"`
}

//ResEval evaluation result abstraction
type ResEval struct {
	Res [][]byte `json:"Result"`
	ComID
}

//PartyInfo container for general information about a node
type PartyInfo struct {
	IP        []byte `json:"IP"`
	Port      int    `json:"Port"`
	PublicKey []byte `json:"PublicKey"`
}

//MyInfo container for general and private information about a node
type MyInfo struct {
	PartyInfo
	PrivateKey []byte `json:"PrivateKey"`
}

//ServerCapabilities server capabilities abstraction
type ServerCapabilities struct {
	NumberOfGates int     `json:"NumberOfGates"`
	FeePerGate    float64 `json:"FeePerGate"`
}

//Token a token container for the ease of message passing
type Token struct {
	TokenGen []byte `json:"TokenGen"`
}

//ServerInfo container for server relevant info in Directory of Service
type ServerInfo struct {
	PartyInfo
	ServerCapabilities
}

//ClientInfo container for client relevant info in Directory of Service
type ClientInfo struct {
	PartyInfo
}

//RegisterationMessage a complete registration message
type RegisterationMessage struct {
	Type   string     `json:"Type"` //Server, Client
	Server ServerInfo `json:"ServerInfo"`
}

//FunctionInfo a container for function requirements
type FunctionInfo struct {
	Token
	NumberOfServers    int `json:"NumberOfServers"`
	ServerCapabilities     //in this case we describe the capabilities needed to compute the circuit
}

//CycleRequestMessage Wrapping In case we needed to add new request types for failure handling
type CycleRequestMessage struct {
	FunctionInfo
}

//Cycle cycle wrapper
type Cycle struct {
	ServersCycle []PartyInfo `json:"ServersCycle"`
}

//CycleMessage a complete cycle message reply
type CycleMessage struct {
	Cycle
	TotalFee int `json:"TotalFee"`
}

//Message passed through cycle
type Message struct {
	Type string `json:"Type"` //Garble, Rerand, Eval
	Circuit
	GarbledMessage
	InputWires []Wire `json:"InputWires"`
	Randomness
	ComID
	NextServer PartyInfo `json:"NextServer"`
}

//MessageArray container of messages
type MessageArray struct {
	Array []Message `json:"Array"`
	Keys  [][]byte  `json:"Keys"`
}

//DirctoryInfo Global Variable to store Directory communication info
var DirctoryInfo = struct {
	port int
	ip   []byte
}{
	port: 0,
	ip:   []byte(""),
}

//MyOwnInfo personal info container
var MyOwnInfo MyInfo

//SetMyInfo sets the info of the current node
func SetMyInfo() {
	pI, sk := GetPartyInfo()
	MyOwnInfo = MyInfo{
		PartyInfo:  pI,
		PrivateKey: sk,
	}
}

//SetDirectoryInfo to set the dircotry info
func SetDirectoryInfo(ip []byte, port int) {
	DirctoryInfo.port = port
	DirctoryInfo.ip = ip
}

//GetCircuitSize get the number of gates in a circuit
func GetCircuitSize(circ Circuit) int {
	return len(circ.InputGates) + len(circ.MiddleGates) + len(circ.OutputGates)
}

//Comm basically, the channel will need to send the input/output mapping as well
func Comm(cir string, cID int64, token Token, numberOfServers int, feePerGate float64, chVDCSCommCircRes chan<- GarbledMessage) {
	file, _ := ioutil.ReadFile(cir + ".json")
	mCirc := Circuit{}
	err := json.Unmarshal([]byte(file), &mCirc) //POSSIBLE BUG
	if err != nil {
		log.Fatal(err)
	}
	rand.Seed(int64(cID))

	circuitSize := GetCircuitSize(mCirc)
	cycleRequestMessage := CycleRequestMessage{
		FunctionInfo{
			Token: Token{
				TokenGen: token.TokenGen,
			},
			NumberOfServers: numberOfServers,
			ServerCapabilities: ServerCapabilities{
				NumberOfGates: circuitSize,
				FeePerGate:    feePerGate,
			},
		},
	}

	cycleMessage, ok := GetFromDirectory(cycleRequestMessage, DirctoryInfo.ip, DirctoryInfo.port)
	for ok == false {
		cycleMessage, ok = GetFromDirectory(cycleRequestMessage, DirctoryInfo.ip, DirctoryInfo.port)
	}

	msgArray, randNess, keys := GenerateMessageArray(cycleMessage, cID, mCirc)
	fmt.Println(cycleMessage)
	fmt.Println(keys) //store the keys somewhere for recovery or pass on channel

	ipS1 := cycleMessage.ServersCycle[0].IP
	portS1 := cycleMessage.ServersCycle[0].Port

	for !SendToServer(msgArray, ipS1, portS1) {

	}

	//Generate input wires
	arrIn, arrOut := GenerateInputWiresValidate(mCirc, randNess, cID)

	//Send Circuit to channel
	var gcm GarbledMessage
	for _, val := range arrIn {
		gcm.InputWires = append(gcm.InputWires, Wire{WireLabel: val})
	}
	for _, val := range arrOut {
		gcm.OutputWires = append(gcm.OutputWires, Wire{WireLabel: val})
	}
	chVDCSCommCircRes <- gcm
}

//GenerateMessageArray Takes a CycleMessage, a cID, and a circuit and creates a message array encrypted and returns it with the corresponding randomness for the user to use
func GenerateMessageArray(cycleMessage CycleMessage, cID int64, circ Circuit) (mArr MessageArray, rArr []Randomness, keys [][]byte) {
	numberOfServers := len(cycleMessage.ServersCycle)

	rArr = GenerateRandomness(numberOfServers, cID)

	message := Message{
		Type:       "Garble",
		Circuit:    circ,
		Randomness: rArr[0],
		ComID:      ComID{CID: strconv.FormatInt(cID, 10)},
		NextServer: cycleMessage.ServersCycle[1],
	}
	k1 := RandomSymmKeyGen()
	messageEnc := EncryptMessageAES(k1, message)

	keys = append(keys, k1)

	k1, err := RSAPublicEncrypt(RSAPublicKeyFromBytes(cycleMessage.ServersCycle[0].PublicKey), k1)
	if err != nil {
		panic("Invalid PublicKey")
	}
	mArr = MessageArray{
		Array: append(mArr.Array, messageEnc),
		Keys:  append(mArr.Keys, k1),
	}

	for i := 1; i < numberOfServers-1; i++ {

		message = Message{
			Type:       "ReRand",
			Randomness: rArr[i],
			ComID:      ComID{CID: strconv.FormatInt(cID, 10)},
			NextServer: cycleMessage.ServersCycle[i+1],
		}

		k1 = RandomSymmKeyGen()
		messageEnc = EncryptMessageAES(k1, message)

		keys = append(keys, k1)

		k1, err = RSAPublicEncrypt(RSAPublicKeyFromBytes(cycleMessage.ServersCycle[i].PublicKey), k1)
		if err != nil {
			panic("Invalid PublicKey")
		}
		mArr = MessageArray{
			Array: append(mArr.Array, messageEnc),
			Keys:  append(mArr.Keys, k1),
		}

	}

	message = Message{
		Type:       "Eval",
		Randomness: rArr[numberOfServers-1],
		ComID:      ComID{CID: strconv.FormatInt(cID, 10)},
		NextServer: MyOwnInfo.PartyInfo,
	}
	k1 = RandomSymmKeyGen()
	messageEnc = EncryptMessageAES(k1, message)

	keys = append(keys, k1)

	k1, err = RSAPublicEncrypt(RSAPublicKeyFromBytes(cycleMessage.ServersCycle[numberOfServers-1].PublicKey), k1)
	if err != nil {
		panic("Invalid PublicKey")
	}
	mArr = MessageArray{
		Array: append(mArr.Array, messageEnc),
		Keys:  append(mArr.Keys, k1),
	}

	return
}

//EncryptMessageAES takes a symmetric key and message, and encrypts the message using that key
func EncryptMessageAES(key []byte, msg Message) Message {
	return msg
}

//RandomSymmKeyGen Generates a random key for the AES algorithm
func RandomSymmKeyGen() (key []byte) {
	return
}

//GenerateInputWiresValidate Given circuit and randomness generate the input wires corresponding to server n-1
func GenerateInputWiresValidate(circ Circuit, rArr []Randomness, cID int64) (in [][]byte, out [][]byte) {
	/*	inputSize := len(mCirc.InputGates) * 2
		outputSize := len(mCirc.OutputGates)

		arrIn := YaoGarbledCkt_in(mCirc.Rin, mCirc.LblLength, inputSize)
		arrOut := YaoGarbledCkt_out(mCirc.Rout, mCirc.LblLength, outputSize)
	*/

	return
}

//GenerateRandomness generates randomness array corresponding to NumberOfServers with a certain computation ID
func GenerateRandomness(numberOfServers int, cID int64) []Randomness {
	//rArr := make(Randomness, numberOfServers)
	/*
		mCirc := CircuitMessage{Circuit: Circuit{
			InputGates:  k.InputGates,
			MiddleGates: k.MiddleGates,
			OutputGates: k.OutputGates,
		},
			ComID: ComID{strconv.Itoa(rand.Int())},
			Randomness: Randomness{Rin: rand.Int63(),
				Rout:      rand.Int63(),
				Rgc:       rand.Int63(),
				LblLength: 16, //Should be rand.Int()%16 + 16
			},
		}*/

	var rArr []Randomness
	return rArr
}

//CompareWires Takes a garbled circuit and compares wires to input,output wires provided by the user
func CompareWires(gcm GarbledMessage, arrIn [][]byte, arrOut [][]byte) bool {
	for k, val := range gcm.InputWires {
		if bytes.Compare(arrIn[k], val.WireLabel) != 0 {
			fmt.Println("I was cheated on this: ", arrIn[k], val.WireLabel)
			//			panic("The server has cheated me") //redo the process, by recovering from panic by recalling comm
			return false
		}
	}
	for k, val := range gcm.OutputWires {
		if bytes.Compare(arrOut[k], val.WireLabel) != 0 {

			fmt.Println("I was cheated on this: ", arrOut[k], val.WireLabel)
			//panic("The server has cheated me") //redo the process, by recovering from panic by recalling comm
			return false
		}
	}
	return true
}

//SendToServer Invokes the post method on the server
func SendToServer(k MessageArray, ip []byte, port int) bool {
	circuitJSON, err := json.Marshal(k)
	req, err := http.NewRequest("POST", "http://"+string(ip)+":"+strconv.Itoa(port)+"/post", bytes.NewBuffer(circuitJSON))
	if err != nil {
		fmt.Println("generating request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	resp.Body.Close()
	if err != nil {
		//log.Fatal(err)
		return false
	}
	return true
}

//GetFromServer Invokes the get method on the server
func GetFromServer(tokenChallenge Token, ip []byte, port int) (token Token, ok bool) {
	ok = false //assume failure
	iDJSON, err := json.Marshal(tokenChallenge)
	req, err := http.NewRequest("GET", "http://"+string(ip)+":"+strconv.Itoa(port)+"/get", bytes.NewBuffer(iDJSON))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &token)
	if err != nil {
		return
	}
	resp.Body.Close()
	ok = true
	return
}

//SendToDirectory Invokes the post method on the directory
func SendToDirectory(k RegisterationMessage, ip []byte, port int) bool {
	circuitJSON, err := json.Marshal(k)
	req, err := http.NewRequest("POST", "http://"+string(ip)+":"+strconv.Itoa(port)+"/post", bytes.NewBuffer(circuitJSON))
	if err != nil {
		fmt.Println("generating request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	resp.Body.Close()
	if err != nil {
		//log.Fatal(err)
		return false
	}
	return true
}

//GetFromDirectory Invokes the get method on the directory
func GetFromDirectory(k CycleRequestMessage, ip []byte, port int) (cyc CycleMessage, ok bool) {
	ok = false //assume failure
	iDJSON, err := json.Marshal(k)
	req, err := http.NewRequest("GET", "http://"+string(ip)+":"+strconv.Itoa(port)+"/get", bytes.NewBuffer(iDJSON))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &cyc)
	if err != nil {
		return
	}
	resp.Body.Close()
	ok = true
	return
}

//SendToClient Invokes the post method on the server
func SendToClient(res ResEval, ip []byte, port int) bool {
	circuitJSON, err := json.Marshal(res)
	req, err := http.NewRequest("POST", "http://"+string(ip)+":"+strconv.Itoa(port)+"/post", bytes.NewBuffer(circuitJSON))
	if err != nil {
		fmt.Println("generating request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	resp.Body.Close()
	if err != nil {
		//log.Fatal(err)
		return false
	}
	return true
}

//GetFromClient Invokes the get method on the client
func GetFromClient(tokenChallenge Token, ip []byte, port int) (token Token, ok bool) {
	ok = false //assume failure
	iDJSON, err := json.Marshal(tokenChallenge)
	req, err := http.NewRequest("GET", "http://"+string(ip)+":"+strconv.Itoa(port)+"/get", bytes.NewBuffer(iDJSON))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &token)
	if err != nil {
		return
	}
	resp.Body.Close()
	ok = true
	return
}

//SendToServerGarble used in pt2
func SendToServerGarble(k CircuitMessage) bool {
	circuitJSON, err := json.Marshal(k)
	req, err := http.NewRequest("POST", "http://localhost:8080/post", bytes.NewBuffer(circuitJSON))
	if err != nil {
		fmt.Println("generating request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	resp.Body.Close()
	if err != nil {
		//log.Fatal(err)
		return false
	}
	return true
}

//GetFromServerGarble used in pt2
func GetFromServerGarble(id string) (k GarbledMessage, ok bool) {
	ok = false //assume failure
	iDJSON, err := json.Marshal(ComID{CID: id})
	req, err := http.NewRequest("GET", "http://localhost:8080/get", bytes.NewBuffer(iDJSON))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &k)
	if err != nil {
		return
	}
	resp.Body.Close()
	if k.CID != id {
		panic("The server sent me the wrong circuit") //replace with a request repeat.
	}
	ok = true
	return
}

//SendToServerEval used in pt2
func SendToServerEval(k GarbledMessage) bool {
	circuitJSON, err := json.Marshal(k)
	req, err := http.NewRequest("POST", "http://localhost:8081/post", bytes.NewBuffer(circuitJSON))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

//GetFromServerEval used in pt2
func GetFromServerEval(id string) (res [][]byte, ok bool) {
	ok = false // assume failure
	iDJSON, err := json.Marshal(ComID{CID: id})
	req, err := http.NewRequest("GET", "http://localhost:8081/get", bytes.NewBuffer(iDJSON))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	var k ResEval
	err = json.Unmarshal(body, &k)
	if err != nil {
		return
	}
	resp.Body.Close()
	if k.CID != id {
		panic("The server sent me the wrong circuit") //replace with a request repeat.
	}
	res = k.Res
	fmt.Println("Result Returned", k.Res)
	ok = true
	return
}

//GenNRandNumbers generating random byte arrays
func GenNRandNumbers(n int, length int, r int64, considerR bool) [][]byte {
	if considerR {
		rand.Seed(r)
	}
	seeds := make([][]byte, n)
	for i := 0; i < n; i++ {
		seeds[i] = make([]byte, length)
		_, err := rand.Read(seeds[i])
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
	}
	return seeds
}

//YaoGarbledCkt_in input wire garbling
func YaoGarbledCkt_in(rIn int64, length int, inputSize int) [][]byte {
	return GenNRandNumbers(2*inputSize, length, rIn, true)
}

//YaoGarbledCkt_out output wire garbling
func YaoGarbledCkt_out(rOut int64, length int, outputSize int) [][]byte {
	// only one output bit for now
	return GenNRandNumbers(2*outputSize, length, rOut, true)
}

//EncryptAES symmetric encryption using AES algorithm
func EncryptAES(encKey []byte, plainText []byte) (ciphertext []byte, ok bool) {

	ok = false //assume failure
	//			encKey = append(encKey, hash)
	c, err := aes.NewCipher(encKey)
	if err != nil {
		//fmt.Println(err)
	}
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		//fmt.Println(err)
		return
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(cryptoRand.Reader, nonce); err != nil {
		//fmt.Println(err)
		return
	}
	ciphertext = gcm.Seal(nonce, nonce, plainText, nil)
	//fmt.Println(ciphertext)
	ok = true

	return
}

//DecryptAES symmetric decryption using AES algorithm
func DecryptAES(encKey []byte, cipherText []byte) (plainText []byte, ok bool) {

	ok = false //assume failure

	c, err := aes.NewCipher(encKey)
	if err != nil {
		//fmt.Println(err)
		return
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		//fmt.Println(err)
		return
	}

	nonceSize := gcm.NonceSize()
	if len(cipherText) < nonceSize {
		//fmt.Println(err)
		return
	}

	nonce, cipherText := cipherText[:nonceSize], cipherText[nonceSize:]
	plainText, err = gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		//fmt.Println(err)
		return
	}
	//fmt.Println(string(plaintext))
	ok = true
	return
}

//Garble circuit garbling
func Garble(circ CircuitMessage) GarbledMessage {

	inputSize := len(circ.InputGates) * 2
	outputSize := len(circ.OutputGates)
	arrIn := YaoGarbledCkt_in(circ.Rin, circ.LblLength, inputSize)
	arrOut := YaoGarbledCkt_out(circ.Rout, circ.LblLength, outputSize)

	inWires := make(map[string][]Wire)
	outWires := make(map[string][]Wire)

	rand.Seed(circ.Rgc)

	var gc GarbledCircuit
	inputWiresGC := []Wire{}
	outputWiresGC := []Wire{}

	gc.CID = circ.CID

	// Input Gates Garbling
	var wInCnt int = 0
	for k, val := range circ.InputGates {
		gc.InputGates = append(gc.InputGates, GarbledGate{
			Gate: Gate{
				GateID: val.GateID,
			},
		})

		gc.InputGates[k].GateInputs = val.GateInputs

		inCnt := int(math.Log2(float64(len(val.TruthTable))))

		//fmt.Printf("%v, %T\n", val.GateID, val.GateID)

		inWires[val.GateID] = []Wire{}

		for i := 0; i < inCnt; i++ {
			inWires[val.GateID] = append(inWires[val.GateID], Wire{
				WireLabel: arrIn[wInCnt],
			}, Wire{
				WireLabel: arrIn[wInCnt+1],
			})
			inputWiresGC = append(inputWiresGC, Wire{
				WireLabel: arrIn[wInCnt],
			}, Wire{
				WireLabel: arrIn[wInCnt+1],
			})
			wInCnt += 2
		}
		outWires[val.GateID] = []Wire{}
		outWire := GenNRandNumbers(2, circ.LblLength, 0, false)
		outWires[val.GateID] = append(outWires[val.GateID], Wire{
			WireLabel: outWire[0],
		}, Wire{
			WireLabel: outWire[1],
		})
		//in1:	0	0	1	1
		//in0:	0	1	0	1
		//out:	1	0	0	1

		//fmt.Println("Here we getting inWires: \n")
		gc.InputGates[k].GarbledValues = make([][]byte, len(val.TruthTable))
		for key, value := range val.TruthTable {
			var concat []byte
			for i := 0; i < inCnt; i++ {
				idx := ((key >> i) & (1))
				concat = append(concat, inWires[val.GateID][(i*2)+idx].WireLabel...)
			}
			concat = append(concat, []byte(val.GateID)...)
			hash := sha256.Sum256(concat)

			var idxOut int
			if value {
				idxOut = 1
			}
			outKey := outWires[val.GateID][int(idxOut)]
			// generate a new aes cipher using our 32 byte long key
			encKey := make([]byte, 32)
			for jk, tmpo := range hash {
				encKey[jk] = tmpo
			}
			var ok bool
			gc.InputGates[k].GarbledValues[key], ok = EncryptAES(encKey, outKey.WireLabel)
			if !ok {
				fmt.Println("Encryption Failed")
			}
		}
		//fmt.Println("\nwe got'em inWires \n")

	}

	//Middle Gates Garbling
	for k, val := range circ.MiddleGates {
		gc.MiddleGates = append(gc.MiddleGates, GarbledGate{
			Gate: Gate{
				GateID: val.GateID,
			},
		})

		gc.MiddleGates[k].GateInputs = val.GateInputs

		inCnt := int(math.Log2(float64(len(val.TruthTable))))

		//fmt.Printf("%v, %T\n", val.GateID, val.GateID)
		inWires[val.GateID] = []Wire{}

		for _, j := range val.GateInputs {
			inWires[val.GateID] = append(inWires[val.GateID], outWires[j][0])
			inWires[val.GateID] = append(inWires[val.GateID], outWires[j][1])
			//wInCnt++
		}
		outWires[val.GateID] = []Wire{}
		outWire := GenNRandNumbers(2, circ.LblLength, 0, false)
		outWires[val.GateID] = append(outWires[val.GateID], Wire{
			WireLabel: outWire[0],
		}, Wire{
			WireLabel: outWire[1],
		})

		gc.MiddleGates[k].GarbledValues = make([][]byte, len(val.TruthTable))
		for key, value := range val.TruthTable {
			//Concatinating the wire labels with the GateID
			var concat []byte
			for i := 0; i < inCnt; i++ {
				idx := ((key >> i) & (1))
				concat = append(concat, inWires[val.GateID][(i*2)+idx].WireLabel...)
			}
			concat = append(concat, []byte(val.GateID)...)

			//Hashing the value
			hash := sha256.Sum256(concat)

			//Determining the value of the output wire
			var idxOut int
			if value {
				idxOut = 1
			}
			outKey := outWires[val.GateID][int(idxOut)]

			// generate a new aes cipher using our 32 byte long key
			encKey := make([]byte, 32)
			for jk, tmpo := range hash {
				encKey[jk] = tmpo
			}
			var ok bool
			gc.MiddleGates[k].GarbledValues[key], ok = EncryptAES(encKey, outKey.WireLabel)
			if !ok {
				fmt.Println("Encryption Failed")
			}
		}

	}

	//Output Gates Garbling
	wOutCnt := 0
	for k, val := range circ.OutputGates {
		gc.OutputGates = append(gc.OutputGates, GarbledGate{
			Gate: Gate{
				GateID: val.GateID,
			},
		})

		gc.OutputGates[k].GateInputs = val.GateInputs

		inCnt := int(math.Log2(float64(len(val.TruthTable))))

		//fmt.Printf("%v, %T\n", val.GateID, val.GateID)

		inWires[val.GateID] = []Wire{}
		for _, j := range val.GateInputs {
			inWires[val.GateID] = append(inWires[val.GateID], outWires[j][0])
			inWires[val.GateID] = append(inWires[val.GateID], outWires[j][1])

			//wInCnt++
		}

		outWires[val.GateID] = []Wire{}

		outWires[val.GateID] = append(outWires[val.GateID], Wire{
			WireLabel: arrOut[wOutCnt],
		}, Wire{
			WireLabel: arrOut[wOutCnt+1],
		})

		outputWiresGC = append(outputWiresGC, Wire{
			WireLabel: arrOut[wOutCnt],
		}, Wire{
			WireLabel: arrOut[wOutCnt+1],
		})
		wOutCnt += 2

		gc.OutputGates[k].GarbledValues = make([][]byte, len(val.TruthTable))
		for key, value := range val.TruthTable {
			var concat []byte
			for i := 0; i < inCnt; i++ {
				idx := ((key >> i) & (1))
				concat = append(concat, inWires[val.GateID][(i*2)+idx].WireLabel...)
			}
			concat = append(concat, []byte(val.GateID)...)
			hash := sha256.Sum256(concat)

			var idxOut int
			if value {
				idxOut = 1
			}
			outKey := outWires[val.GateID][int(idxOut)]
			// generate a new aes cipher using our 32 byte long key
			encKey := make([]byte, 32)
			for jk, tmpo := range hash {
				encKey[jk] = tmpo
			}
			var ok bool
			gc.OutputGates[k].GarbledValues[key], ok = EncryptAES(encKey, outKey.WireLabel)
			if !ok {
				fmt.Println("Encryption Failed")
			}
		}

	}

	//fmt.Println(arrIn)
	//fmt.Println(arrOut)
	//fmt.Println("Input Wires GC:", inWires)
	//fmt.Println("Output Wires GC:", outWires)
	//fmt.Println("GC: ", gc)
	gm := GarbledMessage{
		InputWires:     inputWiresGC,
		GarbledCircuit: gc,
		OutputWires:    outputWiresGC,
	}
	return gm
}

//Evaluate evaluate a garbled circuit
func Evaluate(gc GarbledMessage) (result ResEval) {

	result.CID = gc.CID
	outWires := make(map[string]Wire)
	var wInCnt int

	for _, val := range gc.InputGates {

		inCnt := int(math.Log2(float64(len(val.GarbledValues))))
		var concat []byte
		for i := 0; i < inCnt; i++ {
			concat = append(concat, gc.InputWires[wInCnt].WireLabel...)
			wInCnt++
		}
		concat = append(concat, []byte(val.GateID)...)
		hash := sha256.Sum256(concat)
		encKey := make([]byte, 32)
		for jk, tmpo := range hash {
			encKey[jk] = tmpo
		}
		outWires[val.GateID] = Wire{}
		for _, gValue := range val.GarbledValues {
			tmpWireLabel, ok := DecryptAES(encKey, gValue)
			if ok {
				outWires[val.GateID] = Wire{
					WireLabel: tmpWireLabel,
				}
				break
			}
		}

		if (bytes.Compare(outWires[val.GateID].WireLabel, Wire{}.WireLabel)) == 0 {
			fmt.Println("Fail Evaluation Input Gate")
		} /*else {
			fmt.Println("\n\nYaaay\nGate ", val.GateID, " Now has an output wire: \n", outWires[val.GateID].WireLabel, "\n\n")
		}*/
	}
	for _, val := range gc.MiddleGates {

		//inCnt := len(val.GateInputs)
		var concat []byte
		for _, preGate := range val.GateInputs {
			concat = append(concat, outWires[preGate].WireLabel...)
			//wInCnt++
		}
		concat = append(concat, []byte(val.GateID)...)
		hash := sha256.Sum256(concat)
		encKey := make([]byte, 32)
		for jk, tmpo := range hash {
			encKey[jk] = tmpo
		}
		outWires[val.GateID] = Wire{}
		for _, gValue := range val.GarbledValues {
			tmpWireLabel, ok := DecryptAES(encKey, gValue)
			if ok {
				outWires[val.GateID] = Wire{
					WireLabel: tmpWireLabel,
				}
				break
			}
		}
		if (bytes.Compare(outWires[val.GateID].WireLabel, Wire{}.WireLabel)) == 0 {
			fmt.Println("Fail Evaluation Middle Gate")
		} /*else {
			fmt.Println("\n\nYaaay\nGate ", val.GateID, " Now has an output wire: \n", outWires[val.GateID].WireLabel, "\n\n")
		}*/
	}

	for _, val := range gc.OutputGates {

		//inCnt := len(val.GateInputs)
		var concat []byte
		for _, preGate := range val.GateInputs {
			concat = append(concat, outWires[preGate].WireLabel...)
			//wInCnt++
		}
		concat = append(concat, []byte(val.GateID)...)
		hash := sha256.Sum256(concat)
		encKey := make([]byte, 32)
		for jk, tmpo := range hash {
			encKey[jk] = tmpo
		}
		outWires[val.GateID] = Wire{}
		for _, gValue := range val.GarbledValues {
			tmpWireLabel, ok := DecryptAES(encKey, gValue)
			if ok {
				//fmt.Println("\nI found my way out\n")
				outWires[val.GateID] = Wire{
					WireLabel: tmpWireLabel,
				}
				result.Res = append(result.Res, tmpWireLabel)
				break
			} /*else {
				fmt.Println("\nStill Trying to Find my way out\n")
			}*/
		}
		if (bytes.Compare(outWires[val.GateID].WireLabel, Wire{}.WireLabel)) == 0 {
			fmt.Println("Fail Evaluation Output Gate")
		} /*else {
			fmt.Println("\n\nYaaay\nGate ", val.GateID, " Now has an output wire: \n", outWires[val.GateID].WireLabel, "\n\n")
		}*/
	}

	return
}

//Convert32BytesToByteStream receives a byte array returns the first 32 bytes from it
func Convert32BytesToByteStream(msg [32]byte) []byte {
	key := make([]byte, 32)
	for jk, tmpo := range msg {
		key[jk] = tmpo
	}
	return key
}

//SHA256Hash Hashes a byte array using sha256
func SHA256Hash(msg []byte) [32]byte {
	return sha256.Sum256(msg)
}

// GetIP getting The IP
func GetIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")

	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, err
}

// GetFreePort asks the kernel for a free open port that is ready to use.
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

//RSAPublicEncrypt encrypts data with a given rsa.publickey
func RSAPublicEncrypt(key *rsa.PublicKey, data []byte) ([]byte, error) {
	return rsa.EncryptPKCS1v15(cryptoRand.Reader, key, data)
}

//RSAPrivateDecrypt decrypts encrypted data with a given rsa.privatekey
func RSAPrivateDecrypt(key *rsa.PrivateKey, data []byte) ([]byte, error) {
	return rsa.DecryptPKCS1v15(cryptoRand.Reader, key, data)
}

//GenerateRSAKey generates Public/Private Key pair, advised rsaKeySize = 2048
func GenerateRSAKey(rsaKeySize int) (*rsa.PrivateKey, *rsa.PublicKey) {
	if rsaKeySize < 1 {
		rsaKeySize = 2048
	}
	pri, err := rsa.GenerateKey(cryptoRand.Reader, rsaKeySize)
	if err != nil {
		panic(err)
	}
	return pri, &pri.PublicKey
}

//RSAPublicKeyFromBytes extracts rsa.publickey from its byte array encoding
func RSAPublicKeyFromBytes(key []byte) *rsa.PublicKey {
	pk, err := x509.ParsePKCS1PublicKey(key)
	if err != nil {
		panic(err)
	}
	return pk
}

//BytesFromRSAPublicKey returns byte array encoding from an rsa.publickey
func BytesFromRSAPublicKey(pk *rsa.PublicKey) []byte {
	pubBytes := x509.MarshalPKCS1PublicKey(pk)
	return pubBytes
}

//BytesFromRSAPrivateKey returns byte array encoding from an rsa.privatekey
func BytesFromRSAPrivateKey(sk *rsa.PrivateKey) []byte {
	priBytes, err := x509.MarshalPKCS8PrivateKey(sk)
	if err != nil {
		panic(err)
	}
	return priBytes
}

//RSAPrivateKeyFromBytes extracts rsa.privatekey from its byte array encoding
func RSAPrivateKeyFromBytes(key []byte) *rsa.PrivateKey {
	pri, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		panic(err)
	}
	p, ok := pri.(*rsa.PrivateKey)
	if !ok {
		panic("Invalid Key type")
	}
	return p
}

//RSAPrivateSign makes a signature with a private key
func RSAPrivateSign(key *rsa.PrivateKey, data []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(cryptoRand.Reader, key, crypto.SHA256, Convert32BytesToByteStream(SHA256Hash(data)))
}

//RSAPrivateVerify verifies a signature made with a private key
func RSAPrivateVerify(key *rsa.PrivateKey, sign, data []byte) error {
	h, err := RSAPrivateDecrypt(key, sign)
	if err != nil {
		return err
	}
	if !bytes.Equal(h, Convert32BytesToByteStream(SHA256Hash(data))) {
		return rsa.ErrVerification
	}
	return nil
}

//RSAPublicSign makes a signature with a public key
func RSAPublicSign(key *rsa.PublicKey, data []byte) ([]byte, error) {
	return RSAPublicEncrypt(key, Convert32BytesToByteStream(SHA256Hash(data)))
}

//RSAPublicVerify verifies a signature made with a public key
func RSAPublicVerify(key *rsa.PublicKey, sign, data []byte) error {
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, Convert32BytesToByteStream(SHA256Hash(data)), sign)
}

//GetPartyInfo for a party to extract his own communication info
func GetPartyInfo() (PartyInfo, []byte) {
	port, err := GetFreePort()
	if err != nil {
		panic(err)
	}
	sk, pk := GenerateRSAKey(0)
	if err != nil {
		panic(err)
	}
	ip, err := GetIP()
	if err != nil {
		panic(err)
	}
	pI := PartyInfo{
		IP:        ip,
		Port:      port,
		PublicKey: BytesFromRSAPublicKey(pk),
	}
	return pI, BytesFromRSAPrivateKey(sk)
}
