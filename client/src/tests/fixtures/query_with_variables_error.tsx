import React from "react";
import { Query, QuerySpec } from "../../query";

interface Result {
  output: string;
}

interface Variables {
  y: string;
}

const exampleQuery: QuerySpec<Result, Variables> = {
  query: "query test {}",
};

const withQVOmitted = (
  <Query<Result, Variables> query="test">{data => null}</Query>
);

const withQSOmitted = <Query query={exampleQuery}>{data => null}</Query>;

const withQSEmptyObject = (
  <Query query={exampleQuery} variables={{}}>
    {data => null}
  </Query>
);

const withQVEmptyObject = (
  <Query<Result, Variables> query="test" variables={{}}>
    {data => null}
  </Query>
);
