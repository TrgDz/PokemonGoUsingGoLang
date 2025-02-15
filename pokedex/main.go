package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
)

type Pokemon struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Types []string          `json:"types"`
	Stats map[string]string `json:"stats"`
	EXP   string            `json:"exp"`
}

func main() {
	// Create context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Extend the timeout for our operations to 120 seconds
	ctx, cancel = context.WithTimeout(ctx, 900*time.Second)
	defer cancel()

	var pokemons []Pokemon

	// Navigate and extract data from pokedex.org
	for i := 1; i <= 200; i++ {
		var pokemon Pokemon
		err := chromedp.Run(ctx,
			chromedp.Navigate(fmt.Sprintf("https://pokedex.org/#/pokemon/%d", i)),
			chromedp.Sleep(5*time.Second),
			chromedp.Evaluate(`document.querySelector(".detail-header .detail-national-id").innerText.replace("#", "")`, &pokemon.ID),
			chromedp.Evaluate(`document.querySelector(".detail-panel-header").innerText`, &pokemon.Name),
			chromedp.Evaluate(`Array.from(document.querySelectorAll('.detail-types span.monster-type')).map(elem => elem.innerText)`, &pokemon.Types),
			chromedp.Evaluate(`Object.fromEntries(Array.from(document.querySelectorAll('.detail-stats-row')).map(row => {
				const label = row.querySelector('span:first-child').innerText;
				const value = row.querySelector('.stat-bar-fg').innerText;
				return [label, value];
			}))`, &pokemon.Stats),
			// chromedp.Evaluate(`Object.fromEntries(Array.from(document.querySelectorAll('.when-attacked-row')).map(row => {
			// 	const types = row.querySelectorAll('span.monster-type');
			// 	const multipliers = row.querySelectorAll('span.monster-multiplier');
			// 	return Array.from(types).map((type, index) => {
			// 		const key = type.innerText.trim();
			// 		const value = multipliers[index]?.innerText.trim() || '';
			// 		return key && value ? [key, value] : null;
			// 	}).filter(pair => pair !== null);
			// }).flat())`, &pokemon.DamageMultipliers),
		)
		if err != nil {
			log.Fatalf("Failed to extract data for ID %d: %v", i, err)
		}
		pokemons = append(pokemons, pokemon)
		fmt.Printf("Crawled data for Pokemon ID %d\n", i)
	}

	// Save to JSON file
	file, err := os.Create("pokedex.json")
	if err != nil {
		log.Fatal("Cannot create file", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(pokemons); err != nil {
		log.Fatal("Cannot encode to JSON", err)
	}
}
