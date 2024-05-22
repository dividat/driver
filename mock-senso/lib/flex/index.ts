import * as http from 'http'
import * as ws from 'ws'
import * as Replay from '../replay'

const PORT = 8382 // Driver port

export function mock(replayOpts: Replay.Opts) {
  const server = http.createServer()
  const wss = new ws.Server({ noServer: true })

  wss.on('connection', function connection(ws) {
    const dataStream = Replay.start(replayOpts)
    dataStream.on('data', (data) => ws.send(data))
  })

  server.on('upgrade', function upgrade(request, socket, head) {
    if (request.url === '/flex') {
      wss.handleUpgrade(request, socket, head, function done(ws) {
        wss.emit('connection', ws, request)
      })
    } else {
      socket.destroy()
    }
  })

  server.listen(PORT, () => {
    console.log(`Mocking flex driver at localhost:${PORT}/flex`)
  })
}
