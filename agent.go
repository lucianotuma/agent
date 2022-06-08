package main

/*
TODO:
Criar outra thread pra encaminhar as fotografias tiradas e as informacoes de teclas digitadas e janela ativa para um banco de dados
Verificar um jeito de contabilizar o tempo de janela ativa e como registrar isso no banco de dados.
criar tela de login do funcionário no system tray para verificar jornada de trabalho
criar arquivo de configuração pra nomear a maquina onde esta instalado e definir onde é o banco de dados, sem que isso seja hardcoded

*/
import (
	"encoding/csv"
	"fmt"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"
	"github.com/gin-gonic/gin"
	"github.com/kbinani/screenshot"
	"github.com/kindlyfire/go-keylogger"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/sys/windows"
)

const (
	//constante de delay do loop eterno - em testes parece ser um bom número para capturar toda as teclas sem erro
	delayKeyfetchMS = 50
)

var (
	mod                     = windows.NewLazyDLL("user32.dll")
	procGetWindowText       = mod.NewProc("GetWindowTextW")
	procGetWindowTextLength = mod.NewProc("GetWindowTextLengthW")
	user32                  = syscall.MustLoadDLL("user32.dll")
	kernel32                = syscall.MustLoadDLL("kernel32.dll")
	getLastInputInfo        = user32.MustFindProc("GetLastInputInfo")
	getTickCount            = kernel32.MustFindProc("GetTickCount")
)

var (
	router          *gin.Engine
	nomeFuncionario string
	nomeTerminal    string
)

var loginAuth string

var lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

type (
	HANDLE uintptr
	HWND   HANDLE
)

func terminaPrograma() {
	os.Exit(0)
	//clean up here
}

func onReady() {

	systray.SetTemplateIcon(icon.Data, icon.Data)
	systray.SetTitle("Compass Monitor by STARONE")
	systray.SetTooltip("Compass Monitor está Ativo")
	mQuitOrig := systray.AddMenuItem("Quit", "Sai do Programa")
	mLogarSe := systray.AddMenuItem("Abrir tela de Login", "Clique para efetuar login")
	mDeslogarSe := systray.AddMenuItem("Clique aqui para deslogar o usuário atual", "Clique para deslogar")
	for {
		select {
		case <-mQuitOrig.ClickedCh:
			fmt.Println("Requesting quit")
			systray.Quit()
			fmt.Println("Finished quitting")
			terminaPrograma()
		case <-mLogarSe.ClickedCh:
			open.Run("http://127.0.0.1:8080")
			fmt.Println("Clicou para logar")
		case <-mDeslogarSe.ClickedCh:
			fmt.Println("Clicou para deslogar")
		}
	}

}

func IdleTime() time.Duration {
	lastInputInfo.cbSize = uint32(unsafe.Sizeof(lastInputInfo))
	currentTickCount, _, _ := getTickCount.Call()
	r1, _, err := getLastInputInfo.Call(uintptr(unsafe.Pointer(&lastInputInfo)))
	if r1 == 0 {
		panic("error getting last input info: " + err.Error())
	}
	return time.Duration((uint32(currentTickCount) - lastInputInfo.dwTime)) * time.Millisecond
}

func GetWindowTextLength(hwnd HWND) int {
	ret, _, _ := procGetWindowTextLength.Call(
		uintptr(hwnd))

	return int(ret)
}

func GetWindowText(hwnd HWND) string {
	textLen := GetWindowTextLength(hwnd) + 1

	buf := make([]uint16, textLen)
	procGetWindowText.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(textLen))

	return syscall.UTF16ToString(buf)
}

func getWindow(funcName string) uintptr {
	proc := mod.NewProc(funcName)
	hwnd, _, _ := proc.Call()
	return hwnd
}

func takeScreenshot() {
	n := screenshot.NumActiveDisplays()
	for i := 0; i < n; i++ {
		bounds := screenshot.GetDisplayBounds(i)

		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			panic(err)
		}
		currentTime := time.Now()
		fileName := fmt.Sprintf("Terminal_%d_%dx%d_%s.png", i, bounds.Dx(), bounds.Dy(), currentTime.Format("01.02.2006-15.04.05"))
		file, _ := os.Create(fileName)
		defer file.Close()
		png.Encode(file, img)

		fmt.Printf("#%d : %v \"%s\"\n", i, bounds, fileName)
	}
}

func webServer() {

	// Set the router as the default one provided by Gin
	trmName := systray.AddMenuItem("TERMINAL 1", "Terminal1")
	router = gin.Default()
	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")
	// Define the route for the index page and display the index.html template
	// To start with, we'll use an inline route handler. Later on, we'll create
	// standalone functions that will be used as route handlers.
	router.GET("/", func(c *gin.Context) {
		// Call the HTML method of the Context to render a template
		if loginAuth != "" {
			c.HTML(
				// Set the HTTP status to 200 (OK)
				http.StatusOK,
				// Use the index.html template
				"logado.html",
				// Pass the data that the page uses (in this case, 'title')
				gin.H{
					"title": "Home Page - TESTE",
				},
			)

		} else {
			c.HTML(
				http.StatusOK,
				"login.html",
				gin.H{
					"title": "Home Page - TESTE",
				},
			)
		}
	})
	router.GET("/autentica", func(c *gin.Context) {
		if loginAuth != "" {
			c.HTML(
				http.StatusOK,
				"logado.html",
				gin.H{
					"title": "Home Page - TESTE",
				},
			)

		} else {
			fmt.Println("VARIAVEIS:", c.Query("uname"), c.Query("psw"))
			loginAuth = "1123aaffeawfawef"
			trmName.SetTitle(c.Query("uname"))
			c.HTML(
				http.StatusOK,
				"logado.html",
				gin.H{
					"title": "Home Page - TESTE",
				},
			)

		}
	})

	// Start serving the application
	go router.Run()
}

func realizaRegistro(registros [][]string, arquivoCsv string) {
	f, err := os.OpenFile(arquivoCsv, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln("failed to open file", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	for _, record := range registros {
		if err := w.Write(record); err != nil {
			log.Fatalln("error writing record to file", err)
		}
	}
}

func main() {

	//setup
	nomeFuncionario = "Fulano de Tal"
	nomeTerminal = "Term_012_CoffeShop"
	logRegistroDuracaoTela := make([][]string, 0)
	count := 0
	windowDuration := 0
	kl := keylogger.NewKeylogger()
	var oldForegroundWindowHwnd uintptr
	var windowText string

	onExit := func() {
		now := time.Now()
		ioutil.WriteFile(fmt.Sprintf(`on_exit_%d.txt`, now.UnixNano()), []byte(now.String()), 0644)
	}

	webServer()
	go systray.Run(onReady, onExit)

	for i := 0; true; i += delayKeyfetchMS {
		//fmt.Println(IdleTime())
		// fica capturando a tecla
		key := kl.GetKey()

		// se a tecla for diferente de Empty inclui na contagem
		if !key.Empty {
			count++
		}
		// captura o nome da tela ativa e contabiliza o tempo TODO CUIDADO PARA NÃO ESTOURAR O BUFFER
		if hwnd := getWindow("GetForegroundWindow"); hwnd != 0 && windowText != GetWindowText(HWND(hwnd)) {
			fmt.Println("window :", windowText, "# hwnd:", hwnd, "Duracao Janela:", windowDuration, "oldForeground uintptr:", oldForegroundWindowHwnd)
			if oldForegroundWindowHwnd != 0 {
				logRegistroDuracaoTela = append(logRegistroDuracaoTela, []string{nomeTerminal, nomeFuncionario, windowText, fmt.Sprintf("%d", windowDuration), fmt.Sprintf("%d", hwnd), fmt.Sprintf("%d", time.Now())})
				fmt.Println(logRegistroDuracaoTela)
			}
			windowDuration = 0
			windowText = GetWindowText(HWND(hwnd))
			oldForegroundWindowHwnd = hwnd
		}

		// a cada 72000 ciclos = 100 segundos, (todo: calibrar para ficar aproximadamente aproximadamente 30 minutos), tira um print das telas
		if i >= 7200 {
			i = 0
			go takeScreenshot()
			go realizaRegistro(logRegistroDuracaoTela, "duracaoTela.csv")
			go realizaRegistro([][]string{{nomeTerminal, nomeFuncionario, fmt.Sprintf("%d", count), fmt.Sprintf("%d", time.Now())}}, "teclasApertadas.csv")
			logRegistroDuracaoTela = nil
			count = 0
		}

		//fmt.Printf("Teclas apertadas: %d\r", count)
		windowDuration += delayKeyfetchMS
		//fmt.Println("Window duration:", windowDuration)
		time.Sleep(delayKeyfetchMS * time.Millisecond)
	}
}
