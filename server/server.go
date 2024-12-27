package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// DATA MODELS
// -----------------------------------------------------------------------------

type Pokemon struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Types []string          `json:"types"`
	Stats map[string]string `json:"stats"`
	Exp   string            `json:"exp"`
}

type Player struct {
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	PokeBalls []Pokemon `json:"pokeBalls"`
}

// -----------------------------------------------------------------------------
// GLOBAL VARIABLES
// -----------------------------------------------------------------------------

var (
	// POKEMONS stores all possible Pokemon loaded from pokedex.json
	POKEMONS []Pokemon

	// PLAYERS stores all possible Players loaded from players.json
	PLAYERS []Player

	// BOARD is a 2D grid representing the game map
	ROWS, COLS        = 10, 18
	BOARD             = make([][]string, ROWS)
	POKEMON_LOCATIONS = make(map[string]string) // key: x-y, value: pokemonID
	PLAYER_LOCATIONS  = make(map[string]string) // key: x-y, value: username
	despawnQueues     []string                  // holds queue of x-y coords for despawning pokemons
	CONNECTIONS       = make(map[string]net.Conn)

	// For battle mechanics
	pokeBalls_P1       []Pokemon
	pokeBalls_P2       []Pokemon
	currentDefIndex_P1 int = 0
	currentDefIndex_P2 int = 0
	P1                 string
	P2                 string
	player1Turn        = true
)

// -----------------------------------------------------------------------------
// UTILITY & HELPER FUNCTIONS
// -----------------------------------------------------------------------------

// checkError logs and exits the program on error.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

// isNumber checks if a string can be converted to an integer.
func isNumber(str string) bool {
	_, err := strconv.Atoi(str)
	return err == nil
}

// verifyPlayer checks if a player with given username & password exists.
func verifyPlayer(username, password string, players []Player) bool {
	for _, user := range players {
		if user.Username == username && user.Password == password {
			return true
		}
	}
	return false
}

// loadPlayers loads the list of Players from a local JSON file.
func loadPlayers(filename string) []Player {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening players file: %v\n", err)
		return nil
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("Error reading players file: %v\n", err)
		return nil
	}

	var players []Player
	err = json.Unmarshal(bytes, &players)
	if err != nil {
		fmt.Printf("Error unmarshalling players JSON: %v\n", err)
		return nil
	}

	return players
}

// loadPokemons loads the list of Pokemons from a local JSON file.
func loadPokemons(filename string) []Pokemon {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening pokedex file: %v\n", err)
		return nil
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("Error reading pokedex file: %v\n", err)
		return nil
	}

	var pokemons []Pokemon
	err = json.Unmarshal(bytes, &pokemons)
	if err != nil {
		fmt.Printf("Error unmarshalling pokedex JSON: %v\n", err)
		return nil
	}
	return pokemons
}

// generateRandomPokemons spawns 'num' random Pokemon onto the BOARD.
func generateRandomPokemons(num int) map[string]string {
	pokemonLocations := make(map[string]string)
	for i := 0; i < num; i++ {
		for {
			spawnX := rand.Intn(ROWS)
			spawnY := rand.Intn(COLS)
			if BOARD[spawnX][spawnY] == "" {
				pokemonID := POKEMONS[rand.Intn(len(POKEMONS))].ID
				BOARD[spawnX][spawnY] = pokemonID

				locKey := strconv.Itoa(spawnX) + "-" + strconv.Itoa(spawnY)
				despawnQueues = append(despawnQueues, locKey)
				pokemonLocations[locKey] = pokemonID
				POKEMON_LOCATIONS[locKey] = pokemonID
				break
			}
		}
	}
	return pokemonLocations
}

// handlePokemons runs in its own goroutine to periodically spawn and despawn Pokemon.
func handlePokemons() {
	spawnTicker1min := time.NewTicker(1 * time.Minute)
	despawnTicker5min := time.NewTicker(1 * time.Minute)

	// number of Pokemon to spawn or despawn at a time
	const NUMBERTOPROCESS = 5

	for {
		select {
		case <-spawnTicker1min.C:
			newPokemonLocations, err := json.Marshal(generateRandomPokemons(NUMBERTOPROCESS))
			checkError(err)

			// Notify all connected players about newly spawned Pokemon
			for _, tcpConn := range CONNECTIONS {
				tcpConn.Write(newPokemonLocations)
			}

		case <-despawnTicker5min.C:
			if len(despawnQueues) < NUMBERTOPROCESS {
				continue
			}
			despawnedPokemonLocations := make(map[string]string)
			for i := 0; i < NUMBERTOPROCESS; i++ {
				location := despawnQueues[i]
				despawnedPokemonLocations[location] = ""
				// Clear from BOARD
				coords := strings.Split(location, "-")
				if len(coords) == 2 {
					x, _ := strconv.Atoi(coords[0])
					y, _ := strconv.Atoi(coords[1])
					BOARD[x][y] = ""
				}
				// Remove from POKEMON_LOCATIONS
				delete(POKEMON_LOCATIONS, location)
			}
			despawnQueues = despawnQueues[NUMBERTOPROCESS:]

			// Send these despawns to all players
			sent, _ := json.Marshal(despawnedPokemonLocations)
			for _, tcpConn := range CONNECTIONS {
				tcpConn.Write(sent)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// BATTLE & GAME LOGIC
// -----------------------------------------------------------------------------

// HandleInGameConnection processes movement, catching, and battle data once a user is verified.
func HandleInGameConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		battleStatus := false
		playerMsg, err := reader.ReadString('\n')
		if err != nil {
			// If error, the player has likely disconnected
			removeConnectionAndNotify(conn)
			return
		}

		playerMsg = strings.TrimSpace(playerMsg)
		fmt.Println("Received message:", playerMsg)

		// BATTLE-RELATED PARSING
		if strings.HasPrefix(playerMsg, "battle-") {
			parts := strings.Split(playerMsg, "-")
			if len(parts) < 3 {
				continue
			}
			currentPlayer := parts[1]
			mainMessage := strings.TrimSpace(parts[2])

			// (1) SUBMIT POKEMON
			if isNumber(mainMessage) {
				// The user selected a Pokemon ID to add to his battle team
				submitPokemon(currentPlayer, mainMessage)

				// If both players have selected 3 Pokemon each, we start the battle
				if len(pokeBalls_P1) == 3 && len(pokeBalls_P2) == 3 {
					fmt.Println("Both players have submitted Pokemons. Battle begins!")
					speed_P1, _ := strconv.Atoi(pokeBalls_P1[0].Stats["Speed"])
					speed_P2, _ := strconv.Atoi(pokeBalls_P2[0].Stats["Speed"])
					waitMsg := make(map[string]string)
					waitMsg["battle"] = "wait"
					sentWait, _ := json.Marshal(waitMsg)

					turnMsg := make(map[string]string)
					// Check whose Pokemon is faster
					if speed_P1 >= speed_P2 {
						fmt.Println("P1's turn first")
						turnMsg["battle"] = P1
						sentTurn, _ := json.Marshal(turnMsg)
						CONNECTIONS[P1].Write([]byte(sentTurn))
						CONNECTIONS[P2].Write([]byte(sentWait))
						player1Turn = true
					} else {
						fmt.Println("P2's turn first")
						turnMsg["battle"] = P2
						sentTurn, _ := json.Marshal(turnMsg)
						CONNECTIONS[P2].Write([]byte(sentTurn))
						CONNECTIONS[P1].Write([]byte(sentWait))
						player1Turn = false
					}
				}
			} else {
				// (2) BATTLE ACTIONS (attack, switch, etc.)
				handleBattleAction(currentPlayer, mainMessage)
			}

		} else if strings.HasPrefix(playerMsg, "surrender-") {
			parts := strings.Split(playerMsg, "-")
			winMsg := make(map[string]string)

			if parts[1] == P1 {
				winMsg["battle"] = "victory_" + P2
				sentWin, _ := json.Marshal(winMsg)
				CONNECTIONS[P1].Write([]byte(sentWin))
				CONNECTIONS[P2].Write([]byte(sentWin))
				battleStatus = false
				handleMovementOrEncounter(conn, "4-5", &battleStatus)
			} else {
				winMsg["battle"] = "victory_" + P1
				sentWin, _ := json.Marshal(winMsg)
				CONNECTIONS[P1].Write([]byte(sentWin))
				CONNECTIONS[P2].Write([]byte(sentWin))
				battleStatus = false
				handleMovementOrEncounter(conn, "4-5", &battleStatus)
			}

		} else {
			// MOVEMENT OR ENCOUNTER LOGIC
			handleMovementOrEncounter(conn, playerMsg, &battleStatus)
		}
	}
}

// removeConnectionAndNotify removes the disconnected player's data from global maps
// and notifies all other players of the disconnection.
func removeConnectionAndNotify(conn net.Conn) {
	for username, connection := range CONNECTIONS {
		if connection == conn {
			// Remove player's location
			for loc, player := range PLAYER_LOCATIONS {
				if player == username {
					delete(PLAYER_LOCATIONS, loc)
				}
			}
			// Remove from CONNECTIONS
			delete(CONNECTIONS, username)

			// Broadcast that this player quit
			quitMsg := map[string]string{strings.TrimSpace(username): "quit"}
			sentQuit, _ := json.Marshal(quitMsg)
			for _, otherConn := range CONNECTIONS {
				otherConn.Write(sentQuit)
			}
			fmt.Println(username, "disconnected")
			break
		}
	}
}

// handleMovementOrEncounter deals with the message from a player who wants to move
// or might encounter a Pokemon or another player.
func handleMovementOrEncounter(conn net.Conn, playerCoord string, battleStatus *bool) {
	playerCoord = strings.TrimSpace(playerCoord)

	// Find username from conn
	var thisUsername string
	for name, connection := range CONNECTIONS {
		if connection == conn {
			thisUsername = strings.TrimSpace(name)
			break
		}
	}

	// Remove old location
	for loc, pl := range PLAYER_LOCATIONS {
		if strings.TrimSpace(pl) == thisUsername {
			delete(PLAYER_LOCATIONS, loc)
			break
		}
	}

	// Check if there's a Pokemon at the new location
	if pokemonID, exists := POKEMON_LOCATIONS[playerCoord]; exists {
		// CATCHING
		catchPokemon(conn, thisUsername, playerCoord, pokemonID)
		*battleStatus = true
	} else if enemyName, exists := PLAYER_LOCATIONS[playerCoord]; exists {
		// BATTLE
		initiateBattle(conn, thisUsername, enemyName)
		*battleStatus = true
	}

	// If not battling, update new location
	if !*battleStatus {
		PLAYER_LOCATIONS[playerCoord] = thisUsername
	}

	// Broadcast updated PLAYER_LOCATIONS to all connected players
	broadcastPlayerLocations()
}

// broadcastPlayerLocations sends the entire PLAYER_LOCATIONS map to all players.
func broadcastPlayerLocations() {
	sentPLAYER_LOCATIONS, _ := json.Marshal(PLAYER_LOCATIONS)
	for _, tcpConn := range CONNECTIONS {
		tcpConn.Write([]byte(sentPLAYER_LOCATIONS))
	}
}

// catchPokemon is called when a user steps on a tile with a Pokemon.
func catchPokemon(conn net.Conn, username, locKey, pokemonID string) {
	fmt.Printf("%s is catching Pokemon %s at %s\n", username, pokemonID, locKey)

	// Notify the player that they caught the Pokemon
	caughtMsg := map[string]string{username: pokemonID}
	sentCatched, _ := json.Marshal(caughtMsg)
	conn.Write(sentCatched)
	for i := 0; i < len(PLAYERS); i++ {
		if PLAYERS[i].Username == username {
			pokeID, _ := strconv.Atoi(pokemonID)
			PLAYERS[i].PokeBalls = append(PLAYERS[i].PokeBalls, POKEMONS[pokeID])
		}
	}

	// // Save to JSON file
	file, err := os.Create("players.json")
	if err != nil {
		log.Fatal("Cannot create file", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(PLAYERS); err != nil {
		log.Fatal("Cannot encode to JSON", err)
	}

	// Remove the Pokemon from the board
	coords := strings.Split(locKey, "-")
	if len(coords) == 2 {
		x, _ := strconv.Atoi(coords[0])
		y, _ := strconv.Atoi(coords[1])
		BOARD[x][y] = ""
	}
	delete(POKEMON_LOCATIONS, locKey)

	// Notify other players that the Pokemon is gone
	for _, tcpConn := range CONNECTIONS {
		if tcpConn != conn {
			pokemonGone := map[string]string{locKey: ""}
			sentPokemonGone, _ := json.Marshal(pokemonGone)
			tcpConn.Write([]byte(sentPokemonGone))
		}
	}
}

// initiateBattle sets up a "battle start" scenario between two players.
func initiateBattle(conn net.Conn, thisUsername, enemyUsername string) {
	fmt.Printf("Battle initiated: %s vs %s\n", thisUsername, enemyUsername)

	// Notify the mover
	battleInfo := map[string]string{"battle": enemyUsername}
	sentBattleInfo, _ := json.Marshal(battleInfo)
	conn.Write(sentBattleInfo)

	// Notify the enemy
	battledInfo := map[string]string{"battle": thisUsername}
	sentBattledInfo, _ := json.Marshal(battledInfo)
	CONNECTIONS[enemyUsername].Write(sentBattledInfo)

	// Reset relevant battle data
	pokeBalls_P1 = []Pokemon{}
	pokeBalls_P2 = []Pokemon{}
	P1 = thisUsername
	P2 = enemyUsername
	player1Turn = true
}

// submitPokemon adds the chosen Pokemon to either P1 or P2's team.
func submitPokemon(currentPlayer, pokemonID string) {
	for i := 0; i < len(POKEMONS); i++ {
		if POKEMONS[i].ID == pokemonID {
			if currentPlayer == P1 {
				pokeBalls_P1 = append(pokeBalls_P1, POKEMONS[i])
			} else if currentPlayer == P2 {
				pokeBalls_P2 = append(pokeBalls_P2, POKEMONS[i])
			}
			break
		}
	}
}

// handleBattleAction interprets the action (attack or switch) from the player
// and applies the effect in the battle context.
func handleBattleAction(currentPlayer, mainMessage string) {
	parts := strings.Split(mainMessage, "*")
	if len(parts) != 2 {
		return
	}
	action := strings.TrimSpace(parts[1])

	if action == "switch" {
		if player1Turn {
			currentPokemonIndex, _ := strconv.Atoi(parts[0])
			currentDefIndex_P1 = currentPokemonIndex
		} else {
			currentPokemonIndex, _ := strconv.Atoi(parts[0])
			currentDefIndex_P2 = currentPokemonIndex
		}
	}

	// If it's P1's turn
	if action == "attack" {
		currentPokemonIndex, _ := strconv.Atoi(parts[0])
		if player1Turn {
			if currentPlayer == P1 && action == "attack" {
				// 1) Attack logic
				attackEnemy(pokeBalls_P1, pokeBalls_P2, currentPokemonIndex, currentDefIndex_P2, P2)
				fmt.Print(currentDefIndex_P2)

				// 2) Switch turn to P2

				// 3) Tell the attacker: “Please wait…”
				waitMsg := map[string]string{"battle": "wait"}
				waitJSON, _ := json.Marshal(waitMsg)
				CONNECTIONS[P1].Write([]byte(waitJSON))

				// 4) Tell the defender: “It’s your turn.”
				turnMsg := map[string]string{"battle": P2}
				turnJSON, _ := json.Marshal(turnMsg)
				CONNECTIONS[P2].Write([]byte(turnJSON))

				player1Turn = false

			}
		} else {
			// If it's P2's turn
			if currentPlayer == P2 && action == "attack" {
				// 1) Attack logic
				attackEnemy(pokeBalls_P2, pokeBalls_P1, currentPokemonIndex, currentDefIndex_P1, P1)

				// 2) Switch turn back to P1

				// 3) Tell P2: “Please wait…”
				waitMsg := map[string]string{"battle": "wait"}
				waitJSON, _ := json.Marshal(waitMsg)
				CONNECTIONS[P2].Write([]byte(waitJSON))

				// 4) Tell P1: “It’s your turn.”
				turnMsg := map[string]string{"battle": P1}
				turnJSON, _ := json.Marshal(turnMsg)
				CONNECTIONS[P1].Write([]byte(turnJSON))
				player1Turn = true
			}
		}
	}
}

// attackEnemy applies damage from attackingTeam to defendingTeam.
func attackEnemy(attackingTeam, defendingTeam []Pokemon, attackerIndex, defenderIndex int, defenderPlayer string) {
	if attackerIndex < 0 || attackerIndex >= len(attackingTeam) || len(defendingTeam) == 0 {
		return
	}

	if defenderIndex >= len(defendingTeam) {
		defenderIndex = 0
	}

	defPoke := defendingTeam[defenderIndex]

	// Defensive HP
	defHP, _ := strconv.Atoi(defPoke.Stats["HP"])

	var damage int

	// if specAttackChance == 1 {
	// 	atkValue, _ := strconv.Atoi(attackingTeam[attackerIndex].Stats["Sp Atk"])
	// 	defValue, _ := strconv.Atoi(defPoke.Stats["Sp Def"])
	// 	// Base damage formula: ((2 * Level * Power) / 5 + 2) * (Attack / Defense)
	// 	baseDamage := 50 // Base power
	// 	damage = ((2*50*baseDamage)/5 + 2) * atkValue / defValue
	// 	// Add random factor (85-100%)
	// 	damage = damage * (85 + rand.Intn(16)) / 100
	// } else {
	atkValue, _ := strconv.Atoi(attackingTeam[attackerIndex].Stats["Attack"])
	defValue, _ := strconv.Atoi(defPoke.Stats["Defense"])
	damage = atkValue*50 - defValue
	// }

	// Ensure minimum damage
	if damage < 1 {
		damage = 1
	}

	defHP -= damage

	// Check if Pokemon is defeated
	if defHP <= 0 {
		defHP = 0
		// Remove the fainted Pokemon
		defendingTeam = append(defendingTeam[:defenderIndex], defendingTeam[defenderIndex+1:]...)
	} else {
		// Otherwise, update the local stats with the new HP
		defendingTeam[defenderIndex].Stats["HP"] = strconv.Itoa(defHP)
	}

	// Sync back to the global slice
	if defenderPlayer == P1 {
		pokeBalls_P1 = defendingTeam

	} else {
		pokeBalls_P2 = defendingTeam

	}

	// Notify the defending player about the result
	attackMsg := map[string]string{
		"battle": fmt.Sprintf("attacked-%d-%d-%d", defHP, damage, defenderIndex),
	}
	sentAttackMsg, _ := json.Marshal(attackMsg)
	CONNECTIONS[defenderPlayer].Write([]byte(sentAttackMsg))
	defenderIndex = 0
}

// -----------------------------------------------------------------------------
// CONNECTION HANDLERS
// -----------------------------------------------------------------------------

// update pokeDex
// func updatePokeDex(conn net.Conn, currServerPokeDexNum string) {
// 	infoReader := bufio.NewReader(conn)

// 	// Get number of pokedex
// 	currClientPokeDexNum, err := infoReader.ReadString('\n')
// 	checkError(err)
// 	currClientPokeDexNum = strings.TrimSpace(currClientPokeDexNum)

// 	result := strings.Compare(currClientPokeDexNum, currServerPokeDexNum)

// 	if result == -1 {
// 		conn.Write([]byte(currServerPokeDexNum))
// 	}
// }

// handleAuthConnection handles the initial login/registration flow for a new connection.
func handleAuthConnection(conn net.Conn) {
	infoReader := bufio.NewReader(conn)

	// Get username
	username, err := infoReader.ReadString('\n')
	checkError(err)
	username = strings.TrimSpace(username)

	// Get password
	password, err := infoReader.ReadString('\n')
	checkError(err)
	password = strings.TrimSpace(password)

	// Verify credentials
	if verifyPlayer(username, password, PLAYERS) {
		// If successful, send "successful" to the client
		conn.Write([]byte("successful"))

		// Send some initial Pokemon indexes (3 random indexes for demonstration)
		for i := 0; i < len(PLAYERS); i++ {
			if PLAYERS[i].Username == username {
				loadPokemons := ""
				for j := 0; j < len(PLAYERS[i].PokeBalls); j++ {
					idx := PLAYERS[i].PokeBalls[j].ID
					loadPokemons += idx
					if j < len(PLAYERS[i].PokeBalls)-1 {
						loadPokemons += "-"
					}
				}
				conn.Write([]byte(loadPokemons))
			}
		}

		// Artificial delay (not sure why you put 22 seconds, but preserving)
		time.Sleep(2 * time.Second)

		// Register this connection globally
		CONNECTIONS[username] = conn
		fmt.Println("New player logged in:", username)

		// Send current Pokemon locations
		sendCurrentPokemonLocations(conn)

		// Place player on the BOARD
		placePlayerOnBoard(username)

		// Broadcast updated player locations
		broadcastPlayerLocations()

		// Now handle the rest of the in-game communication
		HandleInGameConnection(conn)

	} else {
		// If failed, send "failed"
		conn.Write([]byte("failed"))
	}
}

// sendCurrentPokemonLocations marshals and sends current Pokemon positions to the client.
func sendCurrentPokemonLocations(conn net.Conn) {
	sentPOKEMON_LOCATIONS, _ := json.Marshal(POKEMON_LOCATIONS)
	conn.Write([]byte(sentPOKEMON_LOCATIONS))
}

// placePlayerOnBoard finds a random empty spot on the BOARD for this player.
func placePlayerOnBoard(username string) {
	for {
		playerX := rand.Intn(ROWS)
		playerY := rand.Intn(COLS)
		if BOARD[playerX][playerY] == "" {
			BOARD[playerX][playerY] = username
			PLAYER_LOCATIONS[fmt.Sprintf("%d-%d", playerX, playerY)] = username
			break
		}
	}
}

// -----------------------------------------------------------------------------
// MAIN FUNCTION
// -----------------------------------------------------------------------------

func main() {
	// Initialize the BOARD
	for i := range BOARD {
		BOARD[i] = make([]string, COLS)
	}

	rand.Seed(time.Now().UnixNano())

	// Load data from JSON
	POKEMONS = loadPokemons("pokedex.json")
	PLAYERS = loadPlayers("players.json")

	// Initial random Pokemon spawn
	generateRandomPokemons(5)
	fmt.Println("Initial Pokemon Locations:", POKEMON_LOCATIONS)

	// Start background goroutine for spawning & despawning Pokemon
	go handlePokemons()

	// Start listening on port 8080
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server is listening on port 8080")

	// Accept new connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}
		// Handle authentication in a new goroutine
		go handleAuthConnection(conn)
	}
}
