// server.js
const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const { v4: uuidv4 } = require('uuid');
const path = require('path');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

app.use(express.static(path.join(__dirname, 'public')));

const hosts = new Map(); // id -> ws

wss.on('connection', (ws) => {
  ws.isAlive = true;
  ws.on('pong', () => ws.isAlive = true);

  ws.on('message', (msg) => {
    try {
      const data = JSON.parse(msg.toString());
      if (data.type === 'register_host') {
        const id = data.id || uuidv4();
        ws.role = 'host';
        ws.id = id;
        hosts.set(id, ws);
        ws.send(JSON.stringify({ type: 'registered', id }));
        console.log('Host registered', id);
        return;
      }
      if (data.type === 'connect_client') {
        const hostWs = hosts.get(data.hostId);
        if (!hostWs) {
          ws.send(JSON.stringify({ type:'error', msg:'host_not_found' }));
          return;
        }
        ws.role = 'client';
        ws.target = data.hostId;
        ws.send(JSON.stringify({ type:'connected' }));
        console.log('Client connected to', data.hostId);
        return;
      }
      if (data.type === 'control') {
        const host = hosts.get(data.hostId);
        if (host && host.readyState === WebSocket.OPEN) {
          host.send(JSON.stringify({ type:'control', payload: data.payload }));
        }
        return;
      }
    } catch (e) {
      // not JSON
    }

    if (ws.role === 'host' && ws.id) {
      wss.clients.forEach(client => {
        if (client.role === 'client' && client.target === ws.id && client.readyState === WebSocket.OPEN) {
          client.send(msg);
        }
      });
    }
  });

  ws.on('close', () => {
    if (ws.role === 'host' && ws.id) {
      hosts.delete(ws.id);
      console.log('Host disconnected', ws.id);
    }
  });
});

setInterval(() => {
  wss.clients.forEach(ws => {
    if (!ws.isAlive) return ws.terminate();
    ws.isAlive = false;
    ws.ping();
  });
}, 30000);

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => console.log('Server listening on', PORT));
