package shared

import (
	"fmt"
	"os"
)

// REQUEST ou REPLY, referente à solicitação de uso da CS
type Message struct {
	PID      int64  // ID do Processo remetente
	Text     string // Texto da mensagem (REPLY ou REQUEST)
	MsgClock int    // No. Ciclo de Clock da mensagem (Ti)
}

func CheckError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

func PrintError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
	}
}
