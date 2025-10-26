import { app, BrowserWindow, ipcMain } from 'electron';
import path from 'path';
import { URL } from 'url';
import { ChunkNetworkManager } from './networking';
import { ConnectionState } from '@shared/protocol';

let mainWindow: BrowserWindow | null = null;
const chunkManager = new ChunkNetworkManager();

function resolveHtmlPath(): string {
  if (process.env.VITE_DEV_SERVER_URL) {
    return process.env.VITE_DEV_SERVER_URL;
  }
  return new URL('../renderer/index.html', `file://${__dirname}/`).toString();
}

async function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 960,
    minHeight: 640,
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false
    }
  });

  const startUrl = resolveHtmlPath();
  await mainWindow.loadURL(startUrl);

  chunkManager.setTarget(mainWindow);

  mainWindow.on('closed', () => {
    mainWindow = null;
    chunkManager.dispose();
  });
}

app.whenReady().then(() => {
  createWindow().catch((err) => {
    console.error('Failed to create window', err);
    app.quit();
  });

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow().catch((err) => {
        console.error('Failed to recreate window', err);
      });
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

ipcMain.handle('join-game', async (_event, urlString: string) => {
  if (!mainWindow) {
    return { ok: false, message: 'Window not initialised yet.' };
  }
  const state: ConnectionState = {
    status: 'connecting',
    message: `Connecting to ${urlString}`
  };
  mainWindow.webContents.send('connection-state', state);
  try {
    const result = await chunkManager.connect(urlString);
    if (!result.ok) {
      const errorState: ConnectionState = {
        status: 'error',
        message: result.message
      };
      mainWindow.webContents.send('connection-state', errorState);
    } else {
      const okState: ConnectionState = {
        status: 'connected',
        message: result.message,
        servers: result.servers
      };
      mainWindow.webContents.send('connection-state', okState);
    }
    return result;
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    const errorState: ConnectionState = {
      status: 'error',
      message
    };
    mainWindow.webContents.send('connection-state', errorState);
    return { ok: false, message };
  }
});
