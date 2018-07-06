import React from "react";
import { Query } from "../query";
import { ThunderProvider } from "../context";
import { Connection } from "../connection";
import { render } from "react-dom";

describe("fixtures", () => {
  test("shouldComponentUpdate", () => {
    const connection = new Connection(
      async () => new WebSocket("ws://localhost"),
    );
    let subscribeCalls = 0;
    connection.subscribe = (() => {
      subscribeCalls++;
      return {
        close: () => {},
        data: () => ({}),
      };
    }) as any;

    const initialProps = { x: 1 };

    const node = document.createElement("div");
    render(
      <ThunderProvider connection={connection}>
        <Query<{}, { x: number }> query={"query"} variables={initialProps}>
          {data => <div />}
        </Query>
      </ThunderProvider>,
      node,
    );

    // newProps are deep equal, but not referentially equal.
    const newProps = { x: 1 };
    render(
      <ThunderProvider connection={connection}>
        <Query<{}, { x: number }> query={"query"} variables={newProps}>
          {data => <div />}
        </Query>
      </ThunderProvider>,
      node,
    );

    expect(subscribeCalls).toStrictEqual(1);

    const newerProps = { x: 2 };
    render(
      <ThunderProvider connection={connection}>
        <Query<{}, { x: number }> query={"query"} variables={newerProps}>
          {data => <div />}
        </Query>
      </ThunderProvider>,
      node,
    );

    expect(subscribeCalls).toStrictEqual(2);
  });
});
