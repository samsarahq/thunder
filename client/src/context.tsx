import React, { createContext } from "react";
import { Connection } from "./connection";

// XXX: Provide a default value that prints an error about
// lack of connection.
export const { Provider, Consumer } = createContext(
  (null as any) as Connection,
);

interface ProviderProps {
  connection: Connection;
}

export const ThunderProvider: React.SFC<ProviderProps> = props => {
  return <Provider value={props.connection}>{props.children}</Provider>;
};
