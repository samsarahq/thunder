export interface Listener {
  onOpen(): void;
  onClose(): void;
  onMessage(envelope: any): void;
}

const initialReconnectDelay = 1000;
const maxReconnectDelay = 30000;

// Connection is a websocket connection that automatically reconnects,
// with exponential backoff.
export class Connection {
  private url: string;
  private listener: Listener;

  private _state: "connected" | "connecting" | "waiting to reconnect" | "closed";
  private socket?: WebSocket;
  private reconnectDelay: number;
  private reconnectHandle?: number;

  constructor(url: string, listener: Listener) {
    this.url = url;
    this.listener = listener;

    this._state = "closed";
    this.socket = undefined;
    this.reconnectDelay = initialReconnectDelay;
    this.reconnectHandle = undefined;

    this.open();
  }

  // state returns the current state of the connection.
  get state() {
    return this._state;
  }

  // open expicitly (re-)opens the connection. If the connection was closed
  // using close() before, the connection will be restored. If the connection
  // was already open, open() will restart the connection.
  open() {
    this.destroySocket();
    this._state = "connecting";

    this.socket = new WebSocket(this.url);
    this.socket.onopen = () => this.onOpen();
    this.socket.onclose = () => this.onClose();
    this.socket.onmessage = (e) => this.onMessage(e);
  }
  
  // close closes the connection. Once the connection is closed, it will no
  // longer attempt to reconnect. After closing the connection, the listener
  // will not receive any more events.
  close() {
    this.destroySocket();
    this._state = "closed";
  }

  // send marshals a message as JSON and sends it over the connection. If the
  // connection isn't open yet, the message will be ignored.
  send(message: any) {
    if (this.socket) {
      this.socket.send(JSON.stringify(message));
    }
  }

  private destroySocket() {
    if (this.socket) {
      this.socket.close();
      this.socket.onopen = () => {};
      this.socket.onclose = () => {};
      this.socket.onmessage = () => {};
      this.socket = undefined;
    }

    if (this.reconnectHandle) {
      clearTimeout(this.reconnectHandle);
      this.reconnectHandle = undefined;
    }
  }

  private onOpen() {
    this._state = "connected";

    // Reset reconnectDelay after a connection stays open for a while.
    this.reconnectHandle = setTimeout(() => { this.reconnectDelay = initialReconnectDelay; }, maxReconnectDelay);

    this.listener.onOpen();
  }

  private onClose() {
    this.destroySocket();

    if (this._state !== "closed") {
      this._state = "waiting to reconnect";

      // Attempt to reconnect after a reconnectDelay, and increase future
      // reconnect delays.
      this.reconnectHandle = setTimeout(() => this.open(), this.reconnectDelay);
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, maxReconnectDelay);
    }

    this.listener.onClose();
  }

  private onMessage(e: MessageEvent) {
    this.listener.onMessage(JSON.parse(e.data));
  }
}
