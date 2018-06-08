import React from "react";
import { Connection } from "./connection";

import { Consumer } from "./context";

interface MutationProps<Result extends object, Input extends object> {
  children: (run: (variables: Input) => Promise<Result>) => React.ReactNode;
  query: string;
  variables: Input;
}

export function Mutation<Result extends object, Input extends object>(
  props: MutationProps<Result, Input>,
) {
  const child = (connection: Connection) => {
    const runMutation = (variables: Input) => {
      return connection.mutate<Input, Result>({
        query: props.query,
        variables,
      });
    };

    return props.children(runMutation);
  };

  return <Consumer>{child}</Consumer>;
}
