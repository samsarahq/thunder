// Options configures a PingingWebsocket.
interface Options {
  // connectionTimeoutMs is the timeout in which the ConnectFunction must
  // succeed and the WebSocket onopen must fire.
  connectionTimeoutMs: number;
  // pingIntervalMs is the delay between a successful ping response and the
  // new ping request.
  pingIntervalMs: number;
  // pingTimeoutMs is the timeout in which a ping response must arrive.
  pingTimeoutMs: number;
}

export type OutEnvelope =
  | {
      type: "subscribe";
      id: string;
      message: { query: string; variables: any };
      extensions?: Record<string, any>;
    }
  | {
      type: "mutate";
      id: string;
      message: { query: string; variables: any };
      extensions?: Record<string, any>;
    }
  | {
      type: "unsubscribe";
      id: string;
    }
  | {
      type: "echo";
    };

export type InEnvelope =
  | {
      type: "update";
      id: string;
      message: any;
      metadata?: Record<string, any>;
    }
  | {
      type: "result";
      id: string;
      message: any;
      metadata?: Record<string, any>;
    }
  | {
      type: "error";
      id: string;
      message: string;
      metadata?: Record<string, any>;
    }
  | {
      type: "echo";
    };

// ConnectFunction is a function that opens a WebSocket. The returned WebSocket
// need not be open yet.
export type ConnectFunction = () => Promise<WebSocket>;

// SocketListener is the interface that PingingWebSocket talks to.
//
// PingingWebSocket guarantees that onOpen, onMessage, and onClose are called in order.
// Furthermore, onOpen is called at most once, and exactly once before any onMessage
// calls, and onClose is called exactly once (even if onOpen was never called).
// once, onMessage will be called only after onOpen, onMessage will not called
// after onClose, and onClose will be called exactly once (even if the
// connection never opened.)
export interface SocketListener {
  onOpen(): void;
  onMessage(message: any): void;
  onClose(reason: string): void;
}

// PingingWebSocket is a WebSocket you can rely on. It has timeouts for all
// operations, and performs its own pings to make sure the connection to the
// backend remains functioning.
export class PingingWebSocket {
  // defaultOptions provides a reasonable set of default PingingWebSocket options.
  static readonly defaultOptions: Options = Object.freeze({
    connectionTimeoutMs: 30000,
    pingIntervalMs: 30000,
    pingTimeoutMs: 30000,
  });

  static readonly reasonPingTimeout = "ping timeout";
  static readonly reasonConnectionTimeout = "connection timeout";
  static readonly reasonCloseCalled = "close called";

  // connectFunction, listener, and options are the PingingWebSocket's configured
  // properties.
  private readonly connectFunction: ConnectFunction;
  private readonly listener: SocketListener;
  private readonly options: Options;

  // socket is the underlying WebSocket. It is non-null after connectFunction
  // returns a promise until hasShutdown is true.
  private socket?: WebSocket;

  // hasShutdown tracks if the PingingWebSocket has been closed for any reason.
  // After it is true, we no longer invoke any methods on the listener, and
  // try not to do any more work.
  private hasShutdown: boolean;

  // connectTimeout is a setTimeout handle for the connection timeout.
  private connectTimeout?: number;
  // sendPingTimeout is a setTimeout handle for sending the next ping. It is
  // undefined while we wait for the next ping.
  private sendPingTimeout?: number;
  // receivePingTimeout is a setTimeout handle for receiving the next ping.
  // It is undefined while we wait to send the next ping.
  private receivePingTimeout?: number;

  constructor(
    connectFunction: ConnectFunction,
    listener: SocketListener,
    options: Partial<Options> = {},
  ) {
    // Initialize our settings and state.
    this.connectFunction = connectFunction;
    this.listener = listener;
    this.options = { ...PingingWebSocket.defaultOptions, ...options };

    this.hasShutdown = false;
  }

  // send sends a message string to the server. If the WebSocket isn't open,
  // send fails silently.
  send(message: OutEnvelope) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      try {
        this.socket.send(JSON.stringify(message));
      } catch (error) {
        console.log(`pingingwebsocket: this.socket.send failed: ${error}`);
      }
    }
  }

  // close closes the PingingWebSocket, calling onClose on the listener if the
  // WebSocket is still open. After close returns, no more functions on the
  // listener will be called.
  close() {
    this.shutdown(PingingWebSocket.reasonCloseCalled);
  }

  // readyState returns the current underlying WebSocket's readyState.
  get readyState(): number {
    if (this.hasShutdown) {
      return WebSocket.CLOSED;
    } else if (this.socket) {
      return this.socket.readyState;
    } else {
      // If we have no socket yet, we are still waiting for the connect
      // promise to complete.
      return WebSocket.CONNECTING;
    }
  }

  async connect() {
    console.log("pingingwebsocket: connecting");

    // Start the connection timeout.
    this.connectTimeout = setTimeout(
      () => this.shutdown(PingingWebSocket.reasonConnectionTimeout),
      this.options.connectionTimeoutMs,
    ) as any;

    // Invoke the connectFunction.
    let socket;
    try {
      socket = await this.connectFunction();
    } catch (error) {
      this.shutdown(`connectFunction: ${error}`);
      return;
    }

    // Catch a shutdown that happened while connectFunction was running.
    if (this.hasShutdown) {
      socket.close();
      return;
    }

    // Store the socket. If we need to close it later, shutdown will do so.
    this.socket = socket;

    // Register socket handlers. All handlers are no-ops once we have shutdown.
    socket.onopen = () => {
      if (this.hasShutdown) {
        return;
      }

      console.log("pingingwebsocket: opened");

      // Clear the timeout now that the connection has opened.
      if (this.connectTimeout) {
        clearTimeout(this.connectTimeout);
        this.connectTimeout = undefined;
      }

      // Start pinging. We will send the first ping after pingIntervalMs.
      this.sendPingTimeout = setTimeout(
        () => this.sendPing(),
        this.options.pingIntervalMs,
      ) as any;

      // Notify the listener.
      this.listener.onOpen();
    };

    socket.onerror = (e: Event) => {
      if (this.hasShutdown) {
        return;
      }

      // Shutdown on error.
      this.shutdown(`socket error: ${e}`);
    };

    socket.onclose = (e: CloseEvent) => {
      if (this.hasShutdown) {
        return;
      }

      // Shutdown on close.
      this.shutdown(`socket close: ${e.reason}`);
    };

    socket.onmessage = (e: MessageEvent) => {
      if (this.hasShutdown) {
        return;
      }

      // Parse the message.
      let envelope: InEnvelope;
      try {
        envelope = JSON.parse(e.data);
      } catch (error) {
        this.shutdown(`socket message JSON.parse: ${error}`);
        return;
      }

      // If it is a ping reply, clear the timeout and schedule a new ping.
      if (envelope.type === "echo") {
        if (this.receivePingTimeout) {
          clearTimeout(this.receivePingTimeout);
          this.receivePingTimeout = undefined;
        }

        if (this.sendPingTimeout) {
          clearTimeout(this.sendPingTimeout);
          this.sendPingTimeout = undefined;
        }
        this.sendPingTimeout = setTimeout(
          () => this.sendPing(),
          this.options.pingIntervalMs,
        ) as any;
      }

      // Notify the listener.
      this.listener.onMessage(envelope);
    };
  }

  // sendPing sends a ping and sets up a timeout for the response.
  private sendPing() {
    if (this.sendPingTimeout) {
      clearTimeout(this.sendPingTimeout);
      this.sendPingTimeout = undefined;
    }

    this.send({ type: "echo" });
    if (this.receivePingTimeout) {
      clearTimeout(this.receivePingTimeout);
      this.receivePingTimeout = undefined;
    }
    this.receivePingTimeout = setTimeout(
      () => this.shutdown(PingingWebSocket.reasonPingTimeout),
      this.options.pingTimeoutMs,
    ) as any;
  }

  // shutdown shuts down the websocket, clears all timeouts and notifies the
  // listener.
  private shutdown(reason: string) {
    // Ensure shutdown is idempotent.
    if (this.hasShutdown) {
      return;
    }
    this.hasShutdown = true;

    console.log(`pingingwebsocket: shutdown: ${reason}`);

    // Shut down the webssocket.
    if (this.socket) {
      try {
        this.socket.close();
      } catch (error) {
        console.log(`pingingwebsocket: this.socket.close failed: ${error}`);
      }
      this.socket = undefined;
    }

    // Clear all timeouts.
    if (this.connectTimeout) {
      clearTimeout(this.connectTimeout);
      this.connectTimeout = undefined;
    }
    if (this.sendPingTimeout) {
      clearTimeout(this.sendPingTimeout);
      this.sendPingTimeout = undefined;
    }
    if (this.receivePingTimeout) {
      clearTimeout(this.receivePingTimeout);
      this.receivePingTimeout = undefined;
    }

    // Notify the listener.
    this.listener.onClose(reason);
  }
}

export default PingingWebSocket;
