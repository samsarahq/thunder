import React from "react";
import { Query } from "../../query";
import { QuerySpec } from "../../spec";

interface Result {
  output: string;
}

interface Variables {
  y: string;
}

const exampleQuery: QuerySpec<Result, Variables> = {
  query: "query test {}",
};

const ComponentWithQueryAndVariables: React.SFC<{}> = () => {
  return (
    <Query<Result, Variables> query="test" variables={{ y: "yes" }}>
      {data => (
        <div>
          {data.state === "subscribed" ? data.value.output : "loading..."}
        </div>
      )}
    </Query>
  );
};

const instanceWithQueryAndVariables = <ComponentWithQueryAndVariables />;

const ComponentWithSpec: React.SFC<{ input: string }> = props => {
  return (
    <Query query={exampleQuery} variables={{ y: props.input }}>
      {data => (
        <div>
          {data.state === "subscribed" ? data.value.output : "loading..."}
        </div>
      )}
    </Query>
  );
};

const instanceWithSpec = <ComponentWithSpec input="s" />;
