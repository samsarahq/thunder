export class BaseConnection {
  constructor(url, options) {
    this.url = url;
    this.options = options;

    // once the connection is closed, it will no longer attempt
    // to reconnect when the socket closes
    this.closed = false;
    this.state = "";
    this.lastStateChange = undefined;

    // reconnectDelay doubles after every failed connection attempt
    this.initialReconnectDelay = 1000;
    this.maxReconnectDelay = 60000;
    this.reconnectDelay = this.initialReconnectDelay;
    this.reconnectHandle = undefined;

    this.open();
  }

  open() {
    // open sets closed to false to undo a previous close
    this.closed = false;
    this.state = "connecting";
    this.lastStateChange = new Date();

    this.destroySocket();

    this.socket = new WebSocket(this.url);
    this.socket.onopen = () => {
      this.state = "connected";
      this.lastStateChange = new Date();

      this.clearReconnectDelay = setTimeout(() => {
        // reset reconnectDelay after a connection stays open for a while
        this.reconnectDelay = this.initialReconnectDelay;
      }, this.maxReconnectDelay);

      this.handleOpen();
    };
    this.socket.onclose = () => this.onClose();
    this.socket.onmessage = (e) => {
      var envelope = JSON.parse(e.data);

      this.handleMessage(envelope);
    }
  }

  destroySocket() {
    if (this.socket) {
      this.socket.close();
      this.socket.onopen = () => {};
      this.socket.onclose = () => {};
      this.socket.onmessage = () => {};
      this.socket.onerror = () => {};
      this.socket = undefined;
    }
  }

  onClose() {
    if (this.clearReconnectDelay !== undefined) {
      clearTimeout(this.clearReconnectDelay);
      this.clearReconnectDelay = undefined;
    }

    if (!this.closed) {
      this.state = "waiting to reconnect";
      this.lastStateChange = new Date();

      this.destroySocket();

      // attempt to reconnect after a reconnectDelay
      this.reconnectHandle = setTimeout(() => {
        this.open();
      }, this.reconnectDelay);

      // increase future reconnectDelay
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
    }

    this.handleClose();
  }

  close() {
    // close sets closed to true to prevent automatic reconnection
    this.closed = true;
    this.state = "closed";
    this.lastStateChange = new Date();

    this.destroySocket();

    if (this.reconnectHandle) {
      clearTimeout(this.reconnectHandle);
      this.reconnectHandle = undefined;
    }
  }

  send(message) {
    if (this.socket) {
      this.socket.send(JSON.stringify(message));
    }
  }

  handleOpen() {}
  handleMessage(envelope) {}
  handleClose() {}
}
