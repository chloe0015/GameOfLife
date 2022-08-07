package gol

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

var turn int

func sendCellFlipped(events chan<- Event, turn int, cell util.Cell) {
	events <- CellFlipped{turn, cell}
}

func mod(n, m int) int {
	return (n + m) % m
}

func calculateNeighbours(height int, width int, world [][]uint8, i int, j int) int {
	count := 0
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			if x != 0 || y != 0 {
				if world[mod(j+x, height)][mod(i+y, width)] == 255 {
					count++
				}
			}
		}
	}
	return count
}

func nextIteration(height, width int, world [][]uint8, c distributorChannels, startY, endY, startX, endX int) [][]uint8 {
	newWorld := make([][]uint8, endY-startY)
	for x := startY; x < endY; x++ {
		newWorld[x-startY] = make([]uint8, endX-startX)
	}
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			neighbours := calculateNeighbours(height, width, world, x, y)
			if neighbours < 2 || neighbours > 3 {
				newWorld[y-startY][x-startX] = 0
				if newWorld[y-startY][x-startX] != world[y][x] {
					cell := util.Cell{Y: y, X: x}
					sendCellFlipped(c.events, turn, cell)
				}

			} else if neighbours == 3 {
				newWorld[y-startY][x-startX] = 255
				if newWorld[y-startY][x-startX] != world[y][x] {
					cell := util.Cell{Y: y, X: x}
					sendCellFlipped(c.events, turn, cell)
				}
			} else {
				newWorld[y-startY][x-startX] = world[y][x]
			}
		}
	}
	return newWorld
}

func sentCellFlipped(newWorld [][]uint8, oldWorld [][]uint8, turn int, c distributorChannels) {
	for i := range oldWorld {
		for j := range oldWorld[i] {
			if oldWorld[i][j] != newWorld[i][j] {
				c.events <- CellFlipped{CompletedTurns: turn,
					Cell: util.Cell{Y: i, X: j}}
			}
		}
	}
}

func MakeCall(client *rpc.Client, StartX, StartY, EndX, EndY, turn, height, width int, id int, world [][]uint8) *stubs.GameOfLifeResponse {
	request := stubs.GameOfLifeRequest{
		StartX: StartX,
		StartY: StartY,
		EndX:   EndX,
		EndY:   EndY,
		Turn:   turn,
		Block:  id,
		World:  world,
		Height: height,
		Width:  width,
	}
	response := new(stubs.GameOfLifeResponse)

	client.Call("Server.HandleGOLRequest", request, response)

	return response
}

func next(client *rpc.Client, world [][]uint8, height, width, startY, endY, startX, endX int, channel chan [][]uint8) {
	b := MakeCall(client, startX, startY, endX, endY, turn, height, width, 1, world)
	channel <- b.World
}

func createChannels(p Params) []chan [][]uint8 {
	channels := make([]chan [][]uint8, p.Threads)
	for i := range channels {
		channels[i] = make(chan [][]uint8)
	}
	return channels
}

func getFinalWorld(channels []chan [][]uint8, p Params) [][]uint8 {
	var finalWorld [][]uint8
	for i := 0; i < p.Threads; i++ {
		b := <-channels[i]
		finalWorld = append(finalWorld, b...)
	}
	return finalWorld
}

func countAliveCells(p Params, world [][]uint8) []util.Cell {
	var cells []util.Cell
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if world[x][y] == 255 {
				cells = append(cells, util.Cell{X: y, Y: x})
			}
		}
	}
	return cells
}

func outputPgm(p Params, c distributorChannels, world [][]uint8, turns int) {
	c.ioCommand <- ioOutput
	outputString := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(turns)
	c.ioFilename <- outputString
	for x := 0; x < p.ImageHeight; x++ {
		for y := 0; y < p.ImageWidth; y++ {
			c.ioOutput <- world[x][y]
		}
	}
	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputString}
}

//TODO ticker
func trackTicker(event chan<- Event, tickerDone chan bool, c distributorChannels, p Params, world *[][]uint8) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			c.events <- AliveCellsCount{turn, len(countAliveCells(p, *world))}
		case <-tickerDone:
			return
		}
	}
}

func keyPresses(p Params, c distributorChannels, world *[][]uint8, mutex *sync.Mutex) {
	for {
		switch <-c.keyPresses {
		case 's':
			outputPgm(p, c, *world, turn)
		case 'q':
			outputPgm(p, c, *world, turn)
			os.Exit(1)
			turn = p.Turns
		case 'p':
			fmt.Println(turn)
			mutex.Lock()
			if <-c.keyPresses == 'p' {
				fmt.Println("Continuing")
				mutex.Unlock()
			}
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Execute all turns of the Game of Life.
	// TODO: Report the final state using FinalTurnCompleteEvent.

	turn = 0
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)
	mutex := &sync.Mutex{}
	tickerDone := make(chan bool)

	//create world
	world := make([][]uint8, p.ImageHeight)

	for x := 0; x < p.ImageWidth; x++ {
		world[x] = make([]uint8, p.ImageHeight)
	}
	//make initial turn
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			world[x][y] = <-c.ioInput
			if world[x][y] == 255 {
				c.events <- CellFlipped{turn, util.Cell{Y: x, X: y}}
			}
		}
	}

	server, err := rpc.Dial("tcp", "localhost:8070")
	if err != nil {
		fmt.Println("dialing:", err)
		return
	}

	server2, err := rpc.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("dialing:", err)
		return
	}

	//server, err := rpc.Dial("tcp", "localhost:8070")
	//if err != nil {
	//	fmt.Println("dialing:", err)
	//	return
	//}

	go trackTicker(c.events, tickerDone, c, p, &world)
	go keyPresses(p, c, &world, mutex)
	//2 servers
	p.Threads = 2
	channels := createChannels(p)
	length := p.ImageHeight / p.Threads
	x := 0
	for x < p.Turns {
		y := 0
		for y < p.Threads-1 {
			go next(server, world, p.ImageHeight, p.ImageWidth, y*length, (y+1)*length, 0, p.ImageWidth, channels[y])
			y++
		}
		go next(server2, world, p.ImageHeight, p.ImageWidth, y*length, p.ImageHeight, 0, p.ImageWidth, channels[p.Threads-1])
		mutex.Lock()
		newWorld := getFinalWorld(channels, p)
		sentCellFlipped(newWorld, world, turn, c)
		world = newWorld

		turn++
		mutex.Unlock()
		c.events <- TurnComplete{turn}
		x++
	}

	//for x := 0; x < p.Turns; x++ {
	//	//calcuate next world
	//	world = nextIteration(p.ImageHeight, p.ImageWidth, world, c, 0, p.ImageHeight, 0, p.ImageWidth)
	//	c.events <- TurnComplete{turn}
	//	turn++
	//}
	tickerDone <- true

	c.events <- FinalTurnComplete{turn, countAliveCells(p, world)}

	outputPgm(p, c, world, p.Turns)

	c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
