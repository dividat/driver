const electron = require('electron');
const shell = electron.shell;
const app = electron.app;
const Menu = electron.Menu;
const Tray = electron.Tray;

const PLAY_URL = 'https://play.dividat.com/';

// Run handler for Windows installer options
if (require('./installation/handle-squirrel-events')()) return;

const Config = require('electron-config');
const config = new Config();
const configKeys = require('./configKeys');

const log = require('electron-log');
log.transports.file.level = 'info';

const pjson = require('../package.json');

let appIcon;

app.on('ready', () => {

    log.info("Dividat Driver starting.");
    log.info("Version: " + pjson.version);

    // load server
    const server = require('./server')();

    const logPath = log.findLogPath('Dividat Driver');

    // Start Play
    if (config.get(configKeys.AUTOSTART_PLAY)) {
        shell.openExternal(PLAY_URL);
    }

    // Create tray
    appIcon = new Tray(__dirname + '/icons/16x16.png');
    const contextMenu = Menu.buildFromTemplate([
        {
            label: 'Play',
            click: () => shell.openExternal(PLAY_URL)
        }, {
            label: 'Always start Play',
            type: 'checkbox',
            checked: config.get(configKeys.AUTOSTART_PLAY),
            click: () => config.set(configKeys.AUTOSTART_PLAY, !config.get(configKeys.AUTOSTART_PLAY))
        }, {
            type: 'separator'
        }, {
            label: 'Version: ' + pjson.version,
            enabled: false
        }, {
            label: "Log",
            click: () => {
                shell.openItem(logPath);
            }
        }, {
            type: 'separator'
        }, {
            label: 'Exit',
            role: 'quit'
        }
    ]);
    appIcon.setToolTip('Dividat Driver');
    appIcon.setContextMenu(contextMenu);
});

// Handle windows ctrl-c...
if (process.platform === "win32") {

    var rl = require("readline").createInterface({input: process.stdin, output: process.stdout});

    rl.on("SIGINT", function() {
        process.emit("SIGINT");
    });
}

process.on("SIGINT", function() {
    //graceful shutdown
    log.info("Caught SIGINT. Closing down.")
    server.close();
    process.exit();
});
