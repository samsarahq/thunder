# Pagination

Thunder supports paginated `FieldFunc`s. When creating a paginated field, you
can either let Thunder handle pagination, or handle pagination yourself.

Our Pagination implementation follows the [Relay spec](https://facebook.github.io/relay/docs/en/pagination-container.html)

## Thunder-managed pagination

The advantage of thunder-managed pagination is that you, as the user, don't have
to do anything. The downside is that Thunder requires the full collection of
resource up-front.

```go
schema := schemabuilder.NewSchema()
object := schema.Object("user", User{})

// A paginated FieldFunc can take arguments just like a normal FieldFunc
object.FieldFunc("addresses", func(ctx context.Context, user *User, args struct {}) ([]*Address, error) {
  // Provide all records to thunder:
  return GetAddresses(ctx, user, args)
}, schemabuilder.Paginated)
```

```graphql
{
  user(id: 5) {
    addresses(
      # Arguments conforming to the Relay spec.
      first: 2,
      after: "",
      # Also available:
      # last: $last,
      # before: $before,
    ) {
      totalCount
      pageInfo {
        hasNextPage
        hasPrevPage
        # Contains pages with page ends and a blank cursor for the first page
        # If I want to go to page N, I can do `after: pages[n-1]`
        pages
        startCursor
        endCursor
      }
      edges {
        node {
          # Contains the full Address
          zipCode
          # ...
        }
        cursor
      }
    }
  }
}
```

```json
{
  "user": {
    "addresses": {
      "totalCount": 50,
      "pageInfo": {
        "hasNextPage": true,
        "hasPrevPage": false,
        "pages": [
          "",
          "Y3Vyc29yMw==",
        ],
        "startCursor": "Y3Vyc29yMw==",
        "endCursor": "Y3xyc39ydw==",
      },
      "edges": [
        {
          "node": {
            "zipCode": "94103"
          },
          "cursor": "Y3Vyc29yMg=="
        },
        {
          "node": {
            "zipCode": "94104"
          },
          "cursor": "Y3Vyc29yMw=="
        }
      ]
    }
  }
}
```

### Sorting

You can define sorts for a field in Thunder. The return value of the sort must
be one of the following (aliases are supported):

- `int`/`int8`/`int16`/`int32`/`int64`
- `uint`/`uint8`/`uint16`/`uint32`/`uint64`
- `float32`/`float64`
- `string`

As you can see below, a sort field does not need to be on the paginated object.

```go
schema := schemabuilder.NewSchema()
object := schema.Object("user", User{})
object.FieldFunc("addresses", func(ctx context.Context, user *User, args struct {}) ([]*Address, error) {
  return GetAddresses(ctx, user, args)
}, schemabuilder.Paginated,
// Each field in this map will be checked when sorting:
schemabuilder.SortFields{
  "houseNumber": func(ctx context.Context, a *Address) int64 {
    return a.Number
  },
  "userName": func(ctx context.Context, a *Address) (string, error) {
    user, err := GetUser(ctx, a.UserId)
    if err != nil {
      return "", err
    }
    return user.Name, nil
  },
})
```

```graphql
{
  user(id: 5) {
    addresses(sortBy: "houseNumber", sortOrder: "asc") {
      totalCount
      edges {
        node { houseNumber }
        cursor
      }
    }
  }
}
```

```json
{
  "user": {
    "addresses": {
      "totalCount": 50,
      "edges": [
        {
          "node": {
            "houseNumber": 1,
          },
          "cursor": "Y3Vyc29yMw=="
        },
        {
          "node": {
            "houseNumber": 2,
          },
          "cursor": "Y3Vyc29yMg=="
        }
      ]
    }
  }
}
```

### Filtering

You can define text filters for one or multiple fields in Thunder.
The return value of a sort field must be a `string`.

As you can see below, a filter field does not need to be on the paginated object.

```go
schema := schemabuilder.NewSchema()
object := schema.Object("user", User{})
object.FieldFunc("addresses", func(ctx context.Context, user *User, args struct {}) ([]*Address, error) {
  return GetAddresses(ctx, user, args)
}, schemabuilder.Paginated,
// Each field in this map will be checked when filtering:
schemabuilder.TextFilterFields{
  "streetAddress": func(ctx context.Context, a *Address) string {
    return a.StreetAddress
  },
  "state": func(ctx context.Context, a *Address) (string, error) {
    state, err := GetState(ctx, a.StateId)
    if err != nil {
      return "", err
    }
    return state.Name, nil
  },
})
```

```graphql
{
  user(id: 5) {
    addresses(filter: "on") {
      totalCount
      edges {
        node {
          streetAddress
          state { name }
        }
        cursor
      }
    }
  }
}
```

```json
{
  "user": {
    "addresses": {
      "totalCount": 2,
      "edges": [
        {
          "node": {
            "streetAddress": "456 Anber St",
            "state": { "name": "Arizona" }
          },
          "cursor": "Y3vyc29DMg=="
        },
        {
          "node": {
            "streetAddress": "123 Daron St",
            "state": { "name": "California" }
          },
          "cursor": "Y3Sycx9DMg=="
        }
      ]
    }
  }
}
```

## User-managed pagination

User-managed pagination supports everything above, but it's managed by _you_.

```go
schema := schemabuilder.NewSchema()
object := schema.Object("user", User{})
object.FieldFunc("addresses", func(ctx context.Context, user *User, args struct {
  PaginatedArgs schemabuilder.PaginatedArgs
}) ([]*Address, schemabuilder.PaginationInfo, error) {
  addresses, pageInfo, err := GetPaginatedAddresses(ctx, user, args)
  addresses, pageInfo, err
}, schemabuilder.Paginated)
```
