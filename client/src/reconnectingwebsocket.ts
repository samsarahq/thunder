import { BasicWebSocket } from "./basicwebsocket";
import {
  ConnectFunction,
  OutEnvelope,
  PingingWebSocket,
  SocketListener,
} from "./pingingwebsocket";

type State = "connecting" | "connected" | "waiting to reconnect" | "closed";

export const reconnectDelayMs = 10000;

export class ReconnectingWebSocket {
  private readonly connectFunction: ConnectFunction;
  private readonly listener: SocketListener;

  private socket?: PingingWebSocket = undefined;
  private reconnectHandle?: number = undefined;
  private hadSuccess: boolean = false;

  constructor(connectFunction: ConnectFunction, listener: SocketListener) {
    this.connectFunction = connectFunction;
    this.listener = listener;
  }

  get state(): State {
    if (this.socket) {
      switch (this.socket.readyState) {
        case BasicWebSocket.CONNECTING:
          return "connecting";
        case BasicWebSocket.OPEN:
          return "connected";
        default:
          return "waiting to reconnect";
      }
    }
    if (this.reconnectHandle) {
      return "waiting to reconnect";
    } else {
      return "closed";
    }
  }

  get ravenTags() {
    return {
      readyState: this.socket && this.socket.readyState,
      state: this.state,
    };
  }

  reconnect = (): Promise<void> => {
    this.destroySocket();
    this.hadSuccess = false;

    this.socket = new PingingWebSocket(this.connectFunction, {
      onOpen: () => {
        this.listener.onOpen();
      },
      onMessage: (envelope: any) => {
        this.hadSuccess = true;
        this.listener.onMessage(envelope);
      },
      onClose: (reason: string) => {
        this.destroySocket();

        // attempt to reconnect after a reconnectDelay
        const waitMs = this.hadSuccess ? 0 : reconnectDelayMs;
        this.reconnectHandle = setTimeout(this.reconnect, waitMs) as any;

        this.listener.onClose(reason);
      },
    });

    return this.socket.connect();
  };

  send(message: OutEnvelope) {
    if (this.socket) {
      this.socket.send(message);
    }
  }

  close() {
    this.destroySocket();
  }

  private destroySocket() {
    if (this.socket) {
      this.socket.close();
      this.socket = undefined;
    }

    if (this.reconnectHandle) {
      clearTimeout(this.reconnectHandle);
      this.reconnectHandle = undefined;
    }
  }
}
