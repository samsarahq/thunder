import React from "react";
import { Query, QuerySpec } from "../../query";

interface Result {
  output: string;
}

const exampleQuery: QuerySpec<Result, undefined> = {
  query: "query test {}",
};

const withQVOmitted = <Query<Result> query="test">{data => null}</Query>;
const withQSOmitted = <Query query={exampleQuery}>{data => null}</Query>;

const withQVUndefined = (
  <Query<Result> query="test" variables={undefined}>
    {data => null}
  </Query>
);
const withQSUndefined = (
  <Query query={exampleQuery} variables={undefined}>
    {data => null}
  </Query>
);
