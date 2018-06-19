import React from "react";
import { Query, QuerySpec } from "../../query";

const exampleQuery: QuerySpec<
  {
    output: string;
  },
  undefined
> = {
  query: "query test {}",
};

const withQVAndEmptyObject = (
  <Query<{}, undefined> query={"test"} variables={{}}>
    {data => null}
  </Query>
);

const withQSAndEmptyObject = (
  <Query query={exampleQuery} variables={{}}>
    {data => null}
  </Query>
);
