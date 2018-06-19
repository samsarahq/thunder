import { Connection } from "../../connection";
import { MutationSpec } from "../../spec";

const connection = new Connection(async () => new WebSocket("fake"));

interface Result {
  output: string;
}

interface Variables {
  y: string;
}

connection.mutate<Variables, Result>({
  query: "test",
  variables: {
    y: "s",
  },
});

const exampleQuery: MutationSpec<Result, Variables> = {
  query: "mutation test {}",
};

connection.mutate<Variables, Result>({
  query: exampleQuery,
  variables: {
    y: "s",
  },
});
