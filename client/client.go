package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "image/png"

	"github.com/chromedp/chromedp"
	"github.com/eiannone/keyboard"
)

// ----------------------------------------------------------------------------------
// GLOBAL VARIABLES & DATA MODELS
// ----------------------------------------------------------------------------------

// Board dimensions
var (
	ROWS, COLS = 10, 18
	BOARD      = make([][]string, ROWS)
)

// Player
var (
	USERNAME       = ""
	X, Y           int
	ENEMIES        = make(map[string]string) // Map from "x-y" -> "enemyUsername"
	DRAWBOARD      = true                    // If true, redraw board
	pokeBalls      []Pokemon                 // All captured Pokemons
	chosenPokemons []Pokemon                 // Pokemons chosen for battle
	currentPokemon = 0                       // Index of currently chosen Pokemon
	returnPokemon  []Pokemon
	isReplay       bool = false
)

// Pokemon struct to match pokedex.json
type Pokemon struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Types []string          `json:"types"`
	Stats map[string]string `json:"stats"`
	Exp   string            `json:"exp"`
}

var POKEMONS []Pokemon // All possible Pokemon loaded from pokedex.json

// ----------------------------------------------------------------------------------
// UTILITY & HELPER FUNCTIONS
// ----------------------------------------------------------------------------------

// checkError prints an error message and terminates the program if err is non-nil.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func clawPokeDex(value int) {
	// Create context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Extend the timeout for our operations to 120 seconds
	ctx, cancel = context.WithTimeout(ctx, 900*time.Second)
	defer cancel()

	var pokemons []Pokemon

	// Navigate and extract data from pokedex.org
	for i := 1; i <= value; i++ {
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
		)
		if err != nil {
			log.Fatalf("Failed to extract data for ID %d: %v", i, err)
		}
		pokemons = append(pokemons, pokemon)
		fmt.Printf("Crawled data for Pokemon ID %d\n", i)
	}

	// Save to JSON file
	file, err := os.Create("./client/pokedex.json")
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

// isNumber checks if a string can be interpreted as an integer.
func isNumber(str string) bool {
	_, err := strconv.Atoi(str)
	return err == nil
}

// loadPokemons loads Pokemon data from a JSON file and returns the slice of Pokemon.
func loadPokemons(filename string) []Pokemon {

	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil
	}

	var pokemons []Pokemon
	err = json.Unmarshal(bytes, &pokemons)
	if err != nil {
		return nil
	}
	return pokemons
}

// clearScreen uses Windows' CLS command to clear the console.
func clearScreen() {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

// drawTitle prints the ASCII Pokemon title logo.
func drawTitle() {
	fmt.Println("                                  ,'\\")
	fmt.Println("    _.----.        ____         ,'  _\\   ___    ___     ____")
	fmt.Println("_,-'       `.     |    |  /`.   \\,-'    |   \\  /   |   |    \\  |`.")
	fmt.Println("\\      __    \\    '-.  | /   `.  ___    |    \\/    |   '-.   \\ |  |")
	fmt.Println(" \\.    \\ \\   |  __  |  |/    ,','_  `.  |          | __  |    \\|  |")
	fmt.Println("   \\    \\/   /,' _`.|      ,' / / / /   |          ,' _`.|     |  |")
	fmt.Println("    \\     ,-'/  / \\ \\    ,'   | \\/ / ,`.|         /  / \\ \\  |     |")
	fmt.Println("     \\    \\ |   \\_/  |   `-.  \\    `'  /|  |    ||   \\_/  | |\\    |")
	fmt.Println("      \\    \\ \\      /       `-.`.___,-' |  |\\  /| \\      /  | |   |")
	fmt.Println("       \\    \\ `.__,'|  |`-._    `|      |__| \\/ |  `.__,'|  | |   |")
	fmt.Println("        \\_.-'       |__|    `-._ |              '-.|     '-.| |   |")
	fmt.Println("                                `'                            '-._|")
}

// drawBoard redraws the current BOARD in ASCII format.
func drawBoard(board [][]string) {
	clearScreen()
	drawTitle()

	// Helper function for horizontal lines
	horizontalLine := func(length int) string {
		return "+" + strings.Repeat("---+", length)
	}

	for _, row := range board {
		fmt.Println(horizontalLine(len(row)))

		for _, cell := range row {
			if cell == "" {
				fmt.Print("|   ")
			} else {
				// Could be a Pokemon ID (numbers) or a Player
				if isNumber(cell) {
					fmt.Printf("| %s ", "?") // Hide numeric ID behind '?'
				} else {
					// It's either me (USERNAME) or an enemy
					if cell == USERNAME {
						fmt.Printf("| %s ", "☻") // My avatar
					} else {
						fmt.Printf("| %s ", "☠") // Another player's avatar
					}
				}
			}
		}
		fmt.Println("|")
	}
	fmt.Println(horizontalLine(len(board[0])))
}

// drawCongrats prints a congrats message (used when you catch a new Pokemon).
func drawCongrats() {
	fmt.Println("░█████╗░░█████╗░███╗░░██╗░██████╗░██████╗░░█████╗░████████╗░██████╗")
	fmt.Println("██╔══██╗██╔══██╗████╗░██║██╔════╝░██╔══██╗██╔══██╗╚══██╔══╝██╔════╝")
	fmt.Println("██║░░╚═╝██║░░██║██╔██╗██║██║░░██╗░██████╔╝███████║░░░██║░░░╚█████╗░")
	fmt.Println("██║░░██╗██║░░██║██║╚████║██║░░╚██╗██╔══██╗██╔══██║░░░██║░░░░╚═══██╗")
	fmt.Println("╚█████╔╝╚█████╔╝██║░╚███║╚██████╔╝██║░░██║██║░░██║░░░██║░░░██████╔╝")
	fmt.Println("░╚════╝░░╚════╝░╚═╝░░╚══╝░╚═════╝░╚═╝░░╚═╝╚═╝░░╚═╝░░░╚═╝░░░╚═════╝░")
}

// drawStats prints a Pokemon’s stats with ASCII bars.
func drawStats(pokemon Pokemon) {
	fmt.Println("Pokemon Name:", pokemon.Name)
	fmt.Printf("Types: %s\n", strings.Join(pokemon.Types, " "))
	fmt.Println()

	// Display each stat as a bar of █
	statsToBar := []string{"HP", "Attack", "Sp Atk", "Defense", "Sp Def", "Speed"}
	for _, stat := range statsToBar {
		val, _ := strconv.Atoi(pokemon.Stats[stat])
		label := stat
		if label == "Sp Atk" {
			label = "SPECIAL ATTACK"
		}
		if label == "Sp Def" {
			label = "SPECIAL DEFENSE"
		}
		fmt.Printf("%-16s ", label+":")
		for i := 0; i < val; i++ {
			fmt.Print("█")
		}
		fmt.Println()
		fmt.Println()
	}
}

// ----------------------------------------------------------------------------------
// FUNCTIONS TO DISPLAY/SHOW NEW POKEMON & BATTLE-RELATED SCENES
// ----------------------------------------------------------------------------------

// showNewPokemon displays for a newly caught Pokemon, plus stats, then
// returns control to the main board.
func showNewPokemon(pokemon Pokemon) {
	// Clear screen and show "congrats" message & stats
	clearScreen()
	drawCongrats()
	drawStats(pokemon)

	// Then show Pokemon image
	//////////////////////////////////////////////////////////////
	//////////////////////////////////////////////////////////////

	// Add this Pokemon to pokeBalls
	pokeBalls = append(pokeBalls, pokemon)

	// Pause a bit
	time.Sleep(2 * time.Second)

	// Redraw the board
	DRAWBOARD = true
	drawBoard(BOARD)
}

// displayDeck merges Pokemon images in sets of 3 (or leftover) and prints them as ASCII
// for a quick “gallery” display.
func displayDeck() {
	clearScreen()
	fmt.Println("LET'S BATTLE!")
	fmt.Println("Here is your Current Pokemons:")

	// We group pokeBalls by 3 for merged images
	for i := 0; i < len(pokeBalls); i++ {

		fmt.Print("\t", i+1)
		fmt.Print(". " + pokeBalls[i].Name)

		// Then show Pokemon image
		//////////////////////////////////////////////////////////////
		//////////////////////////////////////////////////////////////

		fmt.Println()
	}
}

// ----------------------------------------------------------------------------------
// SERVER COMMUNICATION & EVENT HANDLING
// ----------------------------------------------------------------------------------

// readFromServer constantly reads data from the server, parses it, and updates local state.
func readFromServer(conn net.Conn) {
	for {
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			// If there's an error, likely the server closed connection
			fmt.Println("Server disconnected.")
			os.Exit(0)
		}

		data := bytes.TrimSpace(buf[:n])
		if !json.Valid(data) {
			// Try to repair the JSON
			repairedJSON := repairJSON(data)
			if json.Valid(repairedJSON) {
				data = repairedJSON
			} else {
				fmt.Printf("Invalid JSON received: %s", string(data))
				return
			}
		}

		var locations map[string]string

		if err := json.Unmarshal(data, &locations); err != nil {
			switch err := err.(type) {
			case *json.SyntaxError:
				fmt.Printf("Syntax error at byte offset %d: %s", err.Offset, err)
			case *json.UnmarshalTypeError:
				fmt.Printf("Invalid type at byte offset %d: expected=%v got=%v",
					err.Offset, err.Type, err.Value)
			default:
				fmt.Printf("JSON unmarshal error: %v", err)
			}
			return
		}

		// Process the (key=location or command, value=some info) map
		handleServerMessage(conn, locations)
		if DRAWBOARD {
			drawBoard(BOARD)
		}
	}
}

func repairJSON(data []byte) []byte {
	// Remove any trailing }{ patterns
	str := string(data)
	str = strings.Replace(str, "}{", ",", -1)
	return []byte(str)
}

// handleServerMessage goes through each key-value in the server message and acts accordingly.
func handleServerMessage(conn net.Conn, locations map[string]string) {
	for location, id := range locations {
		loc := strings.TrimSpace(location)
		val := strings.TrimSpace(id)

		// 1) BATTLE-RELATED MESSAGES
		if loc == "battle" {
			DRAWBOARD = false
			handleBattleMessage(conn, val)
		} else {
			// 2) MAP UPDATES: Could be Pokemon spawn, player movement, or disconnection
			handleMapUpdate(conn, loc, val)
		}
	}
}

// func handlePokeDexUpdate(conn net.Conn, currPokeDex string) {

// 	conn.Write([]byte(currPokeDex))

// 	infoReader := bufio.NewReader(conn)
// 	serverPokeDexNum, err := infoReader.ReadString('\n')
// 	if err != nil {
// 		fmt.Println("Error reading serverPokeDexNum:", err)
// 		return
// 	}
// 	serverPokeDexNum = strings.TrimSpace(serverPokeDexNum)

// 	fmt.Println("Received serverPokeDexNum:", serverPokeDexNum)

// 	// result := strings.Compare(currPokeDex, serverPokeDexNum)
// 	// if result == -1 {
// 	// 	i, _ := strconv.Atoi(currPokeDex)
// 	// 	clawPokeDex(i)
// 	// }
// }

// handleBattleMessage processes messages that come in with a "battle" key.
func handleBattleMessage(conn net.Conn, message string) {

	if strings.HasPrefix(message, "attacked") {
		// Format: "attacked-HP-DameReceived-Index"
		parts := strings.Split(message, "-")
		damage, _ := strconv.Atoi(parts[2])
		receivedIndex, _ := strconv.Atoi(parts[3])
		if len(parts) >= 3 {
			newHP, _ := strconv.Atoi(parts[1])
			attackedIndex, _ := strconv.Atoi(parts[3])
			if newHP <= 0 && attackedIndex < len(chosenPokemons) {
				if len(chosenPokemons) > 0 {
					clearScreen()
					fmt.Println("You has been attacked!!!")
					fmt.Println(chosenPokemons[receivedIndex].Name, " receive ", damage, " Damage!!!!")
				}
				time.Sleep(2 * time.Second)
				clearScreen()

				// Remove the fainted Pokemon
				chosenPokemons = append(chosenPokemons[:attackedIndex], chosenPokemons[attackedIndex+1:]...)
			} else if attackedIndex < len(chosenPokemons) {
				if len(chosenPokemons) > 0 {
					clearScreen()
					fmt.Println("You has been attacked!!!")
					fmt.Println(chosenPokemons[receivedIndex].Name, " receive ", damage, " Damage!!!!")
				}
				time.Sleep(2 * time.Second)
				clearScreen()

				chosenPokemons[attackedIndex].Stats["HP"] = strconv.Itoa(newHP)
			}
		}

	} else if message == USERNAME {
		// Means it's my turn
		clearScreen()
		fmt.Println("Your turn!")

		isLooping := true
		for isLooping {

			if len(chosenPokemons) == 0 {
				fmt.Println("You have no more Pokemon left!")
				time.Sleep(time.Second)
				conn.Write([]byte("surrender-" + USERNAME + "\n"))
				return
			}

			if currentPokemon == len(chosenPokemons) {
				currentPokemon = 0
			}

			fmt.Println("Alive Pokemons:")
			for i := range chosenPokemons {
				fmt.Printf("%d) %s (HP: %s)\n", i+1, chosenPokemons[i].Name, chosenPokemons[i].Stats["HP"])
			}
			fmt.Printf("\nYou are currently using: %s (HP: %s)\n", chosenPokemons[currentPokemon].Name, chosenPokemons[currentPokemon].Stats["HP"])
			fmt.Println("Choose action: \"1. attack\" or \"2. switch <index>\"")
			fmt.Print("=> ")
			var action string
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				action = scanner.Text()
			}

			if strings.HasPrefix(action, "1") || strings.HasPrefix(action, "attack") {
				conn.Write([]byte("battle-" + USERNAME + "-" + strconv.Itoa(currentPokemon) + "*attack\n"))
				isLooping = false
				break
			} else if strings.HasPrefix(action, "switch") || strings.HasPrefix(action, "2") {
				parts := strings.Split(action, " ")
				if len(parts) == 2 {
					idx, _ := strconv.Atoi(parts[1])
					if idx >= 1 && idx <= len(chosenPokemons) && idx != (currentPokemon+1) {
						currentPokemon = idx - 1
						clearScreen()
						fmt.Println("You switch your pokemon to " + chosenPokemons[currentPokemon].Name + "!")
						conn.Write([]byte("battle-" + USERNAME + "-" + strconv.Itoa(currentPokemon) + "*switch\n"))
					} else if idx >= 1 && idx <= len(chosenPokemons) && idx == currentPokemon+1 {
						clearScreen()
						fmt.Println("You are using this pokemon, please try again!!")
					} else {
						clearScreen()
						fmt.Println("Invalid pokemon, please try again!!")
					}
				} else {
					clearScreen()
					fmt.Println("Your input invalid, please try again!")
				}
			}
		}
	} else if message == "wait" {
		clearScreen()
		fmt.Println("It is your opponent's turn. Please wait...")
	} else if strings.HasPrefix(message, "victory_") {
		parts := strings.Split(message, "_")

		if parts[1] == USERNAME {
			fmt.Println("Congratulation!! You are VICTORY!!")
			pokeBalls = append(returnPokemon, pokeBalls...)
			returnPokemon = nil
			time.Sleep(3 * time.Second)
			clearScreen()
			drawTitle()
			DRAWBOARD = true

			return
		} else {
			fmt.Println("Sorry!! You are Lost, Try Harder next time!!")
			pokeBalls = append(returnPokemon, pokeBalls...)
			returnPokemon = nil
			time.Sleep(3 * time.Second)
			clearScreen()
			drawTitle()
			DRAWBOARD = true

			return
		}
	} else {
		// "message" is the other player's username -> meaning a new battle started
		displayDeck()
		fmt.Println("You are battling against:", message)
		fmt.Println("Select 3 of your Pokemons: ")
		fmt.Println("---------------------------------")

		chosenPokemons = []Pokemon{}
		isColl := false

		for len(chosenPokemons) < 3 {
			fmt.Print("Name: ")
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				continue
			}
			DeckIDSc := scanner.Text()
			var DeckID int
			foundID := false
			foundName := false

			if isNumber(DeckIDSc) {
				DeckID, _ = strconv.Atoi(DeckIDSc)
				DeckID--

				if DeckID >= len(pokeBalls) || DeckID < 0 {
					foundID = false
				} else {
					for _, p := range pokeBalls {
						if pokeBalls[DeckID].Name == p.Name {
							chosenPokemons = append(chosenPokemons, p)
							returnPokemon = append(returnPokemon, p)
							pokeBalls = append(pokeBalls[:DeckID], pokeBalls[DeckID+1:]...)
							// Let the server know which Pokemon ID we’re submitting
							conn.Write([]byte("battle-" + USERNAME + "-" + p.ID + "\n"))
							foundID = true

							clearScreen()
							displayDeck()
							fmt.Println("You are battling against:", message)
							fmt.Println("Select 3 of your Pokemons: ")
							fmt.Println("---------------------------------")
							fmt.Println("You choosed: ")
							for i := range chosenPokemons {
								fmt.Println("\t ", i+1, ". "+chosenPokemons[i].Name)
							}
							break
						}
					}
				}
				time.Sleep(1 * time.Second)
			}
			if isColl {
				fmt.Println("You already chose this Pokemon!")
			}

			if !foundID && !foundName {
				fmt.Println("Your input Pokemon not Found!")
			}
		}
		clearScreen()
		fmt.Println("Waiting for opponent to submit Pokemons...")
	}
}

// handleMapUpdate deals with location-based updates, such as spawning Pokemon,
// moving players, or removing disconnected enemies.
func handleMapUpdate(conn net.Conn, location, val string) {
	// If the "location" field is actually a username (meaning user= "quit"),
	// it indicates a disconnection.
	if val == "quit" {
		fmt.Println(location + " disconnected.")
		// Find & remove them from the board
		for eneLoc, enemy := range ENEMIES {
			if enemy == location {
				coords := strings.Split(eneLoc, "-")
				if len(coords) == 2 {
					ex, _ := strconv.Atoi(coords[0])
					ey, _ := strconv.Atoi(coords[1])
					BOARD[ex][ey] = ""
				}
				delete(ENEMIES, eneLoc)
				break
			}
		}
		return
	}

	// Parse the location from "x-y"
	parts := strings.Split(location, "-")
	if len(parts) != 2 {
		// Not an x-y location, might be the user’s name
		if location == USERNAME && isNumber(val) {
			// Means we just caught a Pokemon with ID=val
			catchIndex, _ := strconv.Atoi(val)
			if catchIndex >= 0 && catchIndex < len(POKEMONS) {
				go showNewPokemon(POKEMONS[catchIndex])
				DRAWBOARD = false
			}
		}
		return
	}

	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])

	// If val is empty, it means the board tile is now cleared
	if val == "" {
		BOARD[x][y] = ""
		return
	}

	// If val is a number, it's a Pokemon ID placed on the board
	if isNumber(val) {
		BOARD[x][y] = val
		return
	}

	// Otherwise, it's a player name (either me or an enemy)
	if val == USERNAME {
		// My position changed
		// Clear old position
		BOARD[X][Y] = ""
		X, Y = x, y
		BOARD[X][Y] = USERNAME
	} else {
		// It's an enemy's movement
		// Remove old location if it existed
		for eneLoc, enemy := range ENEMIES {
			if enemy == val {
				coords := strings.Split(eneLoc, "-")
				if len(coords) == 2 {
					ex, _ := strconv.Atoi(coords[0])
					ey, _ := strconv.Atoi(coords[1])
					BOARD[ex][ey] = ""
				}
				delete(ENEMIES, eneLoc)
				break
			}
		}
		// Update new location
		ENEMIES[location] = val
		BOARD[x][y] = "enemy"
	}
}

// ----------------------------------------------------------------------------------
// MAIN FUNCTION
// ----------------------------------------------------------------------------------

func main() {
	rand.Seed(time.Now().UnixNano())

	// Connect to the server
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		os.Exit(1)
	}

	defer conn.Close()

	// Initialize the board
	for i := range BOARD {
		BOARD[i] = make([]string, COLS)
	}

	// Load all available Pokemons
	POKEMONS = loadPokemons("pokedex.json")
	if len(POKEMONS) == 0 {
		fmt.Println("No Pokemons loaded. Check pokedex.json.")
	}

	// Authentication flow
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Username: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("Password: ")
	scanner.Scan()
	password := scanner.Text()

	// Send username & password
	_, err = conn.Write([]byte(username + "\n"))
	checkError(err)
	_, err = conn.Write([]byte(password + "\n"))
	checkError(err)

	// Get auth response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	checkError(err)

	// If authenticated
	if strings.TrimSpace(string(buf[:n])) == "successful" {

		// Read second message: the 3 random Pokemon indexes
		n, err = conn.Read(buf)
		checkError(err)

		// Possibly: "8-12-41"
		pokemonIndexes := strings.Split(strings.TrimSpace(string(buf[:n])), "-")

		// Show User Pokemon
		for _, idxStr := range pokemonIndexes {
			idx, err := strconv.Atoi(idxStr)
			if err == nil && idx >= 0 && idx < len(POKEMONS) {
				showNewPokemon(POKEMONS[idx-1])
			}
		}

		// Mark the global username
		USERNAME = username

		for !isReplay {

			go readFromServer(conn)

			// Keyboard input for controlling movement
			if err := keyboard.Open(); err != nil {
				fmt.Println("Failed to open keyboard:", err)
				return
			}
			defer keyboard.Close()

			fmt.Println("Use arrow keys to move, ESC to exit.")

			// Main game loop: read keyboard and move around
			for {
				_, key, err := keyboard.GetKey()
				checkError(err)

				switch key {
				case keyboard.KeyArrowUp:
					if X > 0 {
						BOARD[X][Y] = ""
						X--
						BOARD[X][Y] = USERNAME
						_, err := conn.Write([]byte(strconv.Itoa(X) + "-" + strconv.Itoa(Y) + "\n"))
						checkError(err)
					}
				case keyboard.KeyArrowDown:
					if X < ROWS-1 {
						BOARD[X][Y] = ""
						X++
						BOARD[X][Y] = USERNAME
						_, err := conn.Write([]byte(strconv.Itoa(X) + "-" + strconv.Itoa(Y) + "\n"))
						checkError(err)
					}
				case keyboard.KeyArrowLeft:
					if Y > 0 {
						BOARD[X][Y] = ""
						Y--
						BOARD[X][Y] = USERNAME
						_, err := conn.Write([]byte(strconv.Itoa(X) + "-" + strconv.Itoa(Y) + "\n"))
						checkError(err)
					}
				case keyboard.KeyArrowRight:
					if Y < COLS-1 {
						BOARD[X][Y] = ""
						Y++
						BOARD[X][Y] = USERNAME
						_, err := conn.Write([]byte(strconv.Itoa(X) + "-" + strconv.Itoa(Y) + "\n"))
						checkError(err)
					}
				case keyboard.KeyEsc:
					fmt.Println("Exiting game...")
					return
				}
			}

		}

	} else {
		// If authentication failed
		fmt.Println("Login failed. Please check username/password.")
	}
}
