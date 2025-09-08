package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"lab1/shared"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Funções auxiliares para manipulação segura do clock lógico
// Incrementa clock em ação interna
func incrementClock (reason string) int {
	clockMutex.Lock()
	clock++
	fmt.Print("Clock increased:", clock, " | Reason: ", reason, "\n\n")
	t := clock
	clockMutex.Unlock()
	return t
}

// Ao receber request ou reply: atualiza clock para 1+max(clock, recebido)
func updateClockOnReceive(receivedClock int, reason string) {
	clockMutex.Lock()
	if clock < receivedClock {
		clock = receivedClock
	}
	clock++
	fmt.Print("Clock increased:", clock, " | Reason: ", reason, "\n\n")
	clockMutex.Unlock()
}

// Para ler clock atual de forma segura
func getCurrentClock() int {
	clockMutex.Lock()
	t := clock
	clockMutex.Unlock()
	return t
}


// Variáveis globais interessantes para o processo
var err string
var myPort string          // porta do meu servidor
var nServers int           // qtde de outros processo
var CliConn []*net.UDPConn // vetor com conexões para os servidores
// dos outros processos
var ServConn *net.UDPConn // conexão do meu servidor (onde recebo
// mensagens dos outros processos)

// Variáveis de processo
var PID int64 		  // Process ID (único por arquivo)
var clock int         // Clock atual do Ciclo (Ti)
var clockRequest int  // Clock de Request (T)
var clockMutex sync.Mutex
var clockRequestMutex sync.Mutex

// Ti: Clock atual do processo
// T : Tempo de requisição de CS

var pidToIndex map[int64]int    // Tabela de conversão entre índice de um processo em CliConn e seu PID
var MsgQueue []*shared.Message  // Fila de mensagens em caso de REPLY não-imediato
var msgQueueMutex sync.Mutex
var UsingCS bool         // Se o processo está usando CS
var WaitingCS bool       // Se o processo está na fila para usar CS
var RepliesReceived int  // Número de REPLYs recebidas
var usingCSMutex sync.Mutex
var waitingCSMutex sync.Mutex
var repliesReceivedMutex sync.Mutex

func readInput(ch chan string) {
	// Rotina que “escuta” o stdin
	reader := bufio.NewReader(os.Stdin)
	for {
		text, _, _ := reader.ReadLine()
		ch <- string(text)
	}
}

// Recepção de Mensagens (REQUEST/REPLY) de outros processos
func doServerJob() {
	buf := make([]byte, 1024)
	//Loop infinito mesmo
	for {
		// Ler (uma vez somente) da conexão UDP a mensagem
		n, _, err := ServConn.ReadFromUDP(buf)
		shared.PrintError(err)
		var msg shared.Message
		err = json.Unmarshal(buf[:n], &msg)
		shared.PrintError(err)

		if msg.Text == "REQUEST" && PID != msg.PID {
			// Atualiza clock ao receber REQUEST
			updateClockOnReceive(msg.MsgClock, "REQUEST Received")
			
			usingCSMutex.Lock()
			waitingCSMutex.Lock()
			clockMutex.Lock()
			cond1 := !UsingCS && !WaitingCS

			clockRequestMutex.Lock()
			myRequestClock := clockRequest
			clockRequestMutex.Unlock()
			cond2 := WaitingCS && (myRequestClock > msg.MsgClock || (myRequestClock == msg.MsgClock && msg.PID < PID))

			clockMutex.Unlock()
			waitingCSMutex.Unlock()
			usingCSMutex.Unlock()
			if cond1 || cond2 {
				// Envio de REPLY se não estiver usando CS nem na fila
				// ou se tiver na fila mas:
				//	 1. o Relógio Lógico do REQUEST for menor
				//   2. o PID do REQUEST for menor, caso os Relógios sejam iguais
				fmt.Println("REPLY enviado para", msg.PID)
				go doClientJob(pidToIndex[msg.PID], getCurrentClock(), PID, "REPLY")
			} else {
				msgQueueMutex.Lock()
				MsgQueue = append(MsgQueue, &msg)
				msgQueueMutex.Unlock()
				fmt.Println("Na fila: REQUEST de", msg.PID)
			}
		}

		if msg.Text == "REPLY" && PID != msg.PID {
			// Atualiza clock ao receber REPLY
			fmt.Println("\tREPLY recebido de", msg.PID)
			updateClockOnReceive(msg.MsgClock, "REPLY Received")
			repliesReceivedMutex.Lock()
			RepliesReceived++
			repliesReceivedMutex.Unlock()
		}

		repliesReceivedMutex.Lock()
		cond := RepliesReceived == nServers
		repliesReceivedMutex.Unlock()
		if cond {
			// Uso da CS,
			usingCSMutex.Lock()
			UsingCS = true
			usingCSMutex.Unlock()
			waitingCSMutex.Lock()
			WaitingCS = false
			waitingCSMutex.Unlock()

			fmt.Println("Entrei na CS")
			go sendToSR()

			for i := 0; i < 5; i++ {
				fmt.Print("*\n")
				time.Sleep(1 * time.Second)
			}

			UsingCS = false
			fmt.Println("Saí da CS")

			// Limpando as mensagens da fila (dando reply em tudo)
			msgQueueMutex.Lock()
			for _, queuedMsg := range MsgQueue {
				go doClientJob(pidToIndex[queuedMsg.PID], clock, PID, "REPLY")
			}
			MsgQueue = nil
			msgQueueMutex.Unlock()

			repliesReceivedMutex.Lock()
			RepliesReceived = 0
			repliesReceivedMutex.Unlock()
		}
	}
}

// Envia mensagem de uso para a SR, considerando sua porta fixa :10001
func sendToSR() {
	msg := shared.Message{PID: PID, Text: "Using CS", MsgClock: clock}
	jsonMsg, err := json.Marshal(msg)
	shared.PrintError(err)

	// Conectar ao SharedResource na porta 10001
	srAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:10001")
	shared.PrintError(err)
	conn, err := net.DialUDP("udp", nil, srAddr)
	shared.PrintError(err)
	defer conn.Close()

	_, err = conn.Write(jsonMsg)
	shared.PrintError(err)
}

// Envio de Mensagens (REQUEST/REPLY) para outros processos
func doClientJob(otherProcess int, clockReq int, PID int64, text string) {
	msg := shared.Message{PID: PID, Text: text, MsgClock: clockReq}
	jsonMsg, err := json.Marshal(msg)
	shared.PrintError(err)
	_, err = CliConn[otherProcess].Write(jsonMsg)
	shared.PrintError(err)
}

func initConnections() {
	myPort = os.Args[1]
	nServers = len(os.Args) - 2
	/*Esse 2 tira o nome (no caso Process) e tira a primeira porta (que é a minha). As demais portas são dos outros processos*/
	CliConn = make([]*net.UDPConn, nServers)

    // MAP (dict) que relaciona os PIDs aos índices do vetor CliConn
    pidToIndex = make(map[int64]int)
    for i := 0; i < nServers; i++ {
        portStr := strings.TrimPrefix(os.Args[2+i], ":")
        pid, err := strconv.ParseInt(portStr, 10, 64)
        shared.CheckError(err)
        pid = (pid - 10001) // Converte porta para PID (1, 2, 3, ...)
        fmt.Printf("Mapeando PID %d para índice %d\n", pid, i)
        pidToIndex[pid] = i
    }
	fmt.Printf("\n")
	pidInt, err := strconv.ParseInt(strings.TrimPrefix(myPort, ":"), 10, 64)
	shared.CheckError(err)
	PID = pidInt - 10001
	// Dessa forma, PID vale 1, 2, 3, ...

	/*Outros códigos para deixar ok a conexão do meu servidor (onde recebo msgs). O processo já deve ficar habilitado a receber msgs.*/

	ServerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1"+myPort)
	shared.CheckError(err)
	ServConn, err = net.ListenUDP("udp", ServerAddr)
	shared.CheckError(err)

	/*Outros códigos para deixar ok a minha conexão com cada servidor dos outros processos. Colocar tais conexões no vetor CliConn.*/
	for s := 0; s < nServers; s++ {
		ServerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1"+os.Args[2+s])
		shared.CheckError(err)

		/*Aqui não foi definido o endereço do cliente.
		  Usando nil, o próprio sistema escolhe.  */
		Conn, err := net.DialUDP("udp", nil, ServerAddr)
		CliConn[s] = Conn
		shared.CheckError(err)
	}
}

func main() {
	clock = 0
	initConnections()

	//O fechamento de conexões deve ficar aqui, assim só fecha   //conexão quando a main morrer
	defer ServConn.Close()
	for i := 0; i < nServers; i++ {
		defer CliConn[i].Close()
	}

	ch := make(chan string) //canal que guarda itens lidos do teclado
	go readInput(ch)        //chamar rotina que ”escuta” o teclado

	go doServerJob()
	for {
		// Verificar (de forma não bloqueante) se tem algo no
		// stdin (input do terminal)
		select {
		case text, valid := <-ch:
			if valid {
				if string(text) == "x" {
					// input: x
					// Solicita acesso à CS

					usingCSMutex.Lock()
					if UsingCS {
						usingCSMutex.Unlock()
						fmt.Println("\"x\" ignorado. Processo já está usando a CS.")
						continue
					}
					usingCSMutex.Unlock()

					waitingCSMutex.Lock()
					if WaitingCS {
						waitingCSMutex.Unlock()
						fmt.Println("\"x\" ignorado. Processo já está na fila da CS.")
						continue
					}
					WaitingCS = true
					waitingCSMutex.Unlock()
					
					// Incrementa clock antes de enviar requests
					t := incrementClock("REQUEST Sent")
					clockRequestMutex.Lock()
					clockRequest = t
					clockRequestMutex.Unlock()

					repliesReceivedMutex.Lock()
					RepliesReceived = 0
					repliesReceivedMutex.Unlock()

					fmt.Println("Acesso Solicitado aos Processos.")
					for j := 0; j < nServers; j++ {
						clockRequestMutex.Lock()
						go doClientJob(j, clockRequest, PID, "REQUEST")
						clockRequestMutex.Unlock()
					}
				} else if string(PID) == string(text) {
					// input: ID do processo
					// Incremento do relógio lógico
					incrementClock("Own PID input")
				} else {
					// input: Qualquer outra coisa
					// Não é feito nada (ignorar)
					fmt.Println("Input ignorado: \""+string(text)+"\"", string(PID))
				}
			} else {
				fmt.Println("Closed channel!")
			}
		default:
			// Fazer nada!
			// Mas não fica bloqueado esperando o teclado
			time.Sleep(time.Second * 1)
		}

		// Esperar um pouco
		time.Sleep(time.Second * 1)
	}

}
