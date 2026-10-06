// wirtschaftsmodell zeigt, wen welche Zahl der Wirtschaftsregeln wie trifft
// (x/humanity/keeper/wirtschaftsmodell.go, WIRTSCHAFT_ZAHLENPRUEFUNG.md 4.2).
//
//	go run ./cmd/wirtschaftsmodell                          # heutige Zahlen, 12 Monate
//	go run ./cmd/wirtschaftsmodell -setze UmlaufMonat=0.01  # eine Zahl aendern
//	go run ./cmd/wirtschaftsmodell -empfindlichkeit         # jede Zahl halbiert/verdoppelt
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

func main() {
	monate := flag.Int("monate", 12, "so viele Monate rechnen")
	setze := flag.String("setze", "", "Zahlen aendern: Name=Wert[,Name=Wert...] (Namen wie in ModellParameter)")
	empf := flag.Bool("empfindlichkeit", false, "jede Zahl halbiert und verdoppelt gegen alle Akteure zeigen")
	menschen := flag.Int("menschen", 1000, "angenommene Bevoelkerung, auf die der Topf verteilt wuerde")
	flag.Parse()
	if *monate < 1 || *monate > 1200 {
		fmt.Fprintln(os.Stderr, "monate: 1 bis 1200")
		os.Exit(2)
	}
	if *menschen < 0 {
		fmt.Fprintln(os.Stderr, "menschen: nicht negativ")
		os.Exit(2)
	}
	p := keeper.StandardParameter()
	if *setze != "" {
		for _, teil := range strings.Split(*setze, ",") {
			name, wert, ok := strings.Cut(strings.TrimSpace(teil), "=")
			if !ok {
				fmt.Fprintf(os.Stderr, "-setze: %q ist nicht Name=Wert\n", teil)
				os.Exit(2)
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(wert), 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "-setze: %q: %v\n", teil, err)
				os.Exit(2)
			}
			if err := keeper.ModellParameterSetzen(&p, strings.TrimSpace(name), v); err != nil {
				fmt.Fprintln(os.Stderr, "-setze:", err)
				os.Exit(2)
			}
		}
	}
	akteure := keeper.ModellAkteure()
	fmt.Println(keeper.ModellTabelle(keeper.Wirtschaftsmodell(p, akteure, *monate), *menschen))
	if *empf {
		fmt.Println(keeper.ModellEmpfindlichkeit(p, akteure, *monate))
	}
}
