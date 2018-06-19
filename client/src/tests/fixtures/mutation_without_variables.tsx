import React from "react";
import { Mutation, MutationSpec } from "../../mutation";

interface Result {
  output: string;
}

const exampleQuery: MutationSpec<Result, undefined> = {
  query: "mutation test {}",
};

const ComponentWithQueryAndVariables: React.SFC<{}> = () => {
  return (
    <Mutation<Result> query="test">
      {runMutation => (
        <a
          onClick={async () => {
            await runMutation();
          }}
        >
          Do it
        </a>
      )}
    </Mutation>
  );
};

const instanceWithQueryAndVariables = <ComponentWithQueryAndVariables />;

const ComponentWithSpec: React.SFC<{}> = () => {
  return (
    <Mutation query={exampleQuery}>
      {runMutation => (
        <a
          onClick={async () => {
            await runMutation();
          }}
        >
          Do it
        </a>
      )}
    </Mutation>
  );
};

const instanceWithSpec = <ComponentWithSpec />;
