const EventEmitter = require('events');

const router = require('express').Router();

const Connection = require('./persistentConnection');

const led = require('./senso/led');
const motor = require('./senso/motor');

const data = require('./senso/data');

const DATA_PORT = 55568;
const CONTROL_PORT = 55567;
const DEFAULT_SENSO_ADDRESS = '169.254.1.10';

const log = require('electron-log');

let config;
try {
    const Config = require('electron-config');
    config = new Config();
} catch (err) {
    log.warn("Could not load config file.");
    config = {
        get: function(key) {
            return;
        },
        set: function(key, value) {
            return;
        }
    }
}

// // Data logging
// const fs = require('fs');
// const LOG = false;
// if (LOG) {
//     var rawLog = fs.createWriteStream("log_raw.dat");
//     var decodedLog = fs.createWriteStream("log_decoded.csv");
// }

// Pack control Block
function packControl(block) {
    var protocol_header = new Buffer(8);
    protocol_header.fill(0);

    return Buffer.concat([
        protocol_header, block
    ], 8 + block.length)
}

function factory(sensoAddress, recorder) {

    var dataState = data.init()
    var dataEmitter = new EventEmitter();

    let dataConnection;
    let controlConnection;

    var sensoAddress = sensoAddress || config.get('senso.address') || DEFAULT_SENSO_ADDRESS;

    connect(sensoAddress);

    function connect() {

        log.info("SENSO: Connecting to " + sensoAddress);

        // Destroy any previous connections
        if (dataConnection) {
            dataConnection.destroy();
        }

        dataConnection = new Connection(sensoAddress, DATA_PORT, (raw) => {

            // Record data
            if (recorder) {
                recorder.write(raw.toString('base64'));
                recorder.write('\n');
            }

            var dataReturn = data.update(raw, dataState);
            dataState = dataReturn.state;
            dataEmitter.emit('data', dataReturn.toSend);
        }, "DATA");

        if (controlConnection) {
            controlConnection.destroy();
        }
        controlConnection = new Connection(sensoAddress, CONTROL_PORT, (data) => {}, "CONTROL");
    }

    function connected() {
        return (controlConnection.connected && dataConnection.connected);
    }

    function errorMessage() {
        return "Control: " + controlConnection.lastErrorMessage + "; Data: " + dataConnection.lastErrorMessage;
    }

    function sendControl(block) {
        var socket = controlConnection.getSocket();
        if (socket) {
            socket.write(packControl(block));
        }
    }

    return {
        onWS: function(ws) {

            log.info("WS: Connected.");

            function send(data) {
                ws.send(JSON.stringify(data), function(err) {
                    if (err) {
                        log.warn("WS: Error sending data, " + err);
                    }
                });
            }

            dataEmitter.on('data', send);

            // handle disconnect
            ws.on('close', function close() {
                log.info("WS: Disconnected.")
                dataEmitter.removeListener('data', send);
            });

            // Handle incomming messages
            ws.on('message', function(data, flags) {
                var msg = JSON.parse(data);
                switch (msg.type) {
                    case "Led":
                        var s = msg.setting;
                        var ledBlock = led(s.channel, s.symbol, s.mode, s.color, s.brightness, s.power);
                        sendControl(ledBlock);
                        break;

                    case "Motor":
                        var s = msg.setting;
                        var motorBlock = motor(s.channel, s.mode, s.impulses, s.impulse_duration);
                        sendControl(motorBlock);
                        break;

                    case "Connect":
                        sensoAddress = msg.address;
                        config.set("senso.address", sensoAddress);
                        connect();
                        break;

                    default:
                        log.warn("CONTROL: Unkown control message type from Play: " + msg.type);
                        break;

                }
            });

        },

        router: router.get('/', function(req, res) {
            res.json({address: sensoAddress, connected: connected(), error: errorMessage()});
        })
    };

}

module.exports = factory;
