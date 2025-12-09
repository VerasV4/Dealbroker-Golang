package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// --- CONFIGURAÇÕES ---
const (
	SpreadsheetID   = "1zGQjh1s50gQBlEoEFJiRs5ERfYYqRxSI_YxN4asgQU8"
	TargetRange     = "dealBroker!A2:I"
	CredentialsFile = "docs-api-call-d7e6aa8d3712.json"
	TargetURL       = "https://brokers.mktlab.app/signin"
	LoginEmail      = "jp.azevedo@v4company.com"
	LoginPass       = "Peedriinho459!"
	WhatsappURL     = "https://api.zapsterapi.com/v1/wa/messages"
)

// Estrutura do Lead
type Lead struct {
	Nome          string `json:"nome"`
	Tipo          string `json:"tipo"`
	Faturamento   string `json:"faturamento"`
	Segmento      string `json:"segmento"`
	Produto       string `json:"produto"`
	Canal         string `json:"canal"`
	Preco         string `json:"preco"`
	TempoRestante string `json:"tempo_restante"`
}

func main() {
	ctxSheets := context.Background()
	sheetsService, err := sheets.NewService(ctxSheets, option.WithCredentialsFile(CredentialsFile))
	if err != nil {
		log.Fatalf("Erro ao iniciar Google Sheets: %v", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	log.Println("Iniciando navegação e login...")
	err = chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, LoginEmail, chromedp.ByID),
		chromedp.SendKeys(`#password`, LoginPass, chromedp.ByID),
		chromedp.Click(`button[type='submit']`, chromedp.NodeVisible),
		chromedp.WaitVisible(`[data-testid="cards-grid"]`, chromedp.ByQuery),
	)
	if err != nil {
		log.Fatalf("Falha no login: %v", err)
	}
	log.Println("Login realizado com sucesso!")

	for {
		log.Println("Verificando cards...")

		var leads []Lead
		err := chromedp.Run(ctx,
			chromedp.Evaluate(extractionJS, &leads),
		)

		if err != nil {
			log.Printf("Erro na extração (possível refresh): %v", err)
			chromedp.Run(ctx, chromedp.Reload())
			time.Sleep(5 * time.Second)
			continue
		}

		if len(leads) > 0 {
			processLeads(sheetsService, leads)
		} else {
			log.Println("Nenhum card encontrado.")
		}

		log.Println("Aguardando 5s para próxima verificação...")
		time.Sleep(5 * time.Second)

		chromedp.Run(ctx, chromedp.Reload())
		chromedp.Run(ctx, chromedp.WaitVisible(`[data-testid="cards-grid"]`, chromedp.ByQuery))
	}
}

func processLeads(srv *sheets.Service, leads []Lead) {
	// Ler dados existentes
	resp, err := srv.Spreadsheets.Values.Get(SpreadsheetID, TargetRange).Do()
	if err != nil {
		log.Printf("Erro ao ler planilha: %v", err)
		return
	}

	existingNames := make(map[string]bool)
	if len(resp.Values) > 0 {
		for _, row := range resp.Values {
			if len(row) > 1 {
				existingNames[fmt.Sprintf("%v", row[1])] = true
			}
		}
	}

	var newRows [][]interface{}
	timestamp := time.Now().Format("02/01/2006 15:04")

	for _, lead := range leads {
		if _, exists := existingNames[lead.Nome]; !exists {
			log.Printf("Novo Lead detectado: %s", lead.Nome)

			go sendWhatsapp(lead)

			// Preparar linha
			row := []interface{}{
				timestamp, lead.Nome, lead.Tipo, lead.Segmento,
				lead.Faturamento, lead.Produto, lead.Canal,
				lead.Preco, lead.TempoRestante,
			}
			newRows = append(newRows, row)
		}
	}

	if len(newRows) > 0 {
		rb := &sheets.ValueRange{Values: newRows}
		_, err := srv.Spreadsheets.Values.Append(SpreadsheetID, TargetRange, rb).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			log.Printf("Erro ao salvar no Sheets: %v", err)
		} else {
			log.Printf("Sucesso! %d novos registros salvos.", len(newRows))
		}
	}
}

func sendWhatsapp(lead Lead) {
	score := 0
	if strings.Contains(lead.Faturamento, "400 mil") || strings.Contains(lead.Faturamento, "milhões") {
		score = 30
	}

	stars := "⭐️"
	if score >= 30 {
		stars = "⭐️⭐️⭐️"
	}

	msg := fmt.Sprintf(`*Novo %s Detectado! %s*
Nome: %s
Segmento: %s
Faturamento: %s
Valor Atual: %s
Tempo Restante: %s

*Detalhes:*
- Produto: %s
- Canal: %s`,
		lead.Tipo, stars, lead.Nome, lead.Segmento, lead.Faturamento,
		lead.Preco, lead.TempoRestante, lead.Produto, lead.Canal)

	payload := map[string]string{
		"recipient":   "group:120363371362817488",
		"text":        msg,
		"instance_id": "h3t9n2wmfmne9i2c4s5s6",
	}
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", WhatsappURL, strings.NewReader(string(jsonPayload)))
	req.Header.Add("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3MzczNzI4MzksImlzcyI6InphcHN0ZXJhcGkiLCJzdWIiOiI5NjgyZjg2NC1jMDc5LTQ0YjMtYjhkMC1lOWEzZGJjZTU2MTgiLCJqdGkiOiI3ZjlkMjg2MS04MjdmLTQ2NDQtOGIyZS1kMWQyZTdkNjM3MWQifQ.XBo9IITzeVidiPEV6VPuMdBI-bj7jZ-c_BkKnEGwoLI")
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	_, err := client.Do(req)
	if err != nil {
		log.Printf("Erro WhatsApp: %v", err)
	}
}

// --- JAVASCRIPT DE EXTRAÇÃO ---
const extractionJS = `
(function() {
    const cards = document.querySelectorAll('div[data-testid="auction-card"]');
    const results = [];

    cards.forEach(card => {
        // Helpers
        const getText = (selector) => {
            const el = card.querySelector(selector);
            return el ? el.innerText.replace(/\n/g, ' ').trim() : "N/A";
        };
        
        const getByLabel = (label) => {
            const spans = card.getElementsByTagName('span');
            for (let s of spans) {
                if (s.textContent === label) {
                    return s.nextElementSibling ? s.nextElementSibling.textContent.trim() : "N/A";
                }
            }
            return "N/A";
        };

        // Nome
        let nome = "Desconhecido";
        const pTitle = card.querySelector("p[title]");
        if (pTitle) nome = pTitle.getAttribute("title");

        // Tipo (Ajustado para ser mais genérico e pegar qualquer tag colorida no topo)
        let tipo = "Lead"; // Valor padrão caso falhe
        // Tenta pegar o texto do badge colorido (roxo ou verde geralmente)
        const tipoDiv = card.querySelector("div.rounded-md span.text-neutral-50") || 
                        card.querySelector("div[class*='bg-'] span.text-xs");
        if (tipoDiv) tipo = tipoDiv.innerText;

        results.push({
            nome: nome,
            tipo: tipo,
            faturamento: getByLabel("Faturamento"),
            segmento: getByLabel("Segmento"),
            produto: getByLabel("Tipo de produto"),
            canal: getByLabel("Canal"),
            preco: getText("div.rounded-bl-xl"),
            tempo_restante: getText("div.rounded-br-xl span.tabular-nums")
        });
    });
    return results;
})()
`
