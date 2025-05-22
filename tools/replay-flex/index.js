// Mock the driver at localhost:8382 to replay Senso Flex package recordings

const argv = require('minimist')(process.argv.slice(2))
const fs = require('fs')
const split = require('binary-split')
const websocket = require('ws')
const http = require('http')
const EventEmitter = require('events')

var recFile = argv['_'].pop() || 'rec/flex/zero.dat'
let speedFactor = 1/(parseFloat(argv['speed']) || 1)
let driverVersion = argv['driverVersion'] || "9.9.9-REPLAY"
let loop = !argv['once']

// Create a never ending stream of data
function Replayer (recFile) {
  var emitter = new EventEmitter()

  function createStream () {
    var stream = new fs.createReadStream(recFile).pipe(split())

    stream.on('data', (data) => {
      stream.pause()

      var items = data.toString().split(',')
      var msg
      var timeout
      if (items.length === 2) {
        msg = items[1]
        timeout = items[0]
      } else {
        msg = items[0]
        timeout = 20
      }
      var buf = Buffer.from(msg, 'base64')
      emitter.emit('data', buf)

      setTimeout(() => {
        stream.resume()
      }, timeout * speedFactor)
    }).on('end', () => {
      if (loop) {
        console.log('End of the record stream, looping.')
        createStream()
      } else {
        console.log('End of the record stream, exiting.')
        process.exit(0)
      }
    })
  }
  createStream()
  return emitter
}

const driverMetadata = {
    "message":   "Dividat Driver",
    "version":   driverVersion,
    "machineId": "b58f4aa6e34227c2d0517c924c9060bc8a25d8de677bb42d9dd3d9d2a7eb128d",
    "os":        "linux",
    "arch":      "amd64",
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/') {
    res.writeHead(200, {'Content-Type': 'application/json'});
    res.writeHead(200, {'Access-Control-Allow-Origin': '*'});
    res.end(JSON.stringify(driverMetadata));
  } else {
    res.writeHead(404);
    res.end();
  }
});

const wss = new websocket.Server({ server });

wss.on('connection', function connection(ws) {
  const dataStream = Replayer(recFile)
  dataStream.on('data', (data) => ws.send(data))
});

server.listen(8382, () => {
  console.log('Mock Driver running at http://localhost:8382/');
});
