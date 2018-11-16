# Changelog

## [Unreleased]

### Changed

#### `graphql`

- Object key must now be scalar. ([#190](https://github.com/samsarahq/thunder/pull/190))
- `ErrorCause` is a new exported function that can be used to unwrap pathErrors returned from middlleware. ([#191](https://github.com/samsarahq/thunder/pull/191))
- `FieldFunc` now supports Pagination option, `PaginateFieldFunc` is deprecated. ([#197](https://github.com/samsarahq/thunder/pull/197))

#### `livesql`

- Support filter types that serialize into `[]byte`. ([#172](https://github.com/samsarahq/thunder/pull/172))
- Serializes `sqlgen.Tester` into protobuf message.

#### `reactive`

- `reactive.AddDependency` accepts a serializable object to be added to dependency set tracker. ([#165](https://github.com/samsarahq/thunder/pull/165))

#### `sqlgen`

- `sqlgen.Tester` now compares `driver.Value`s. ([#170](https://github.com/samsarahq/thunder/pull/170))
- Support converting the zero value of fields to NULL in the db with tag `sql:",implicitnull"`. ([#181](https://github.com/samsarahq/thunder/pull/181))
- Support non-pointer protobuf structs. ([#185](https://github.com/samsarahq/thunder/pull/185))
- `BuildStruct` is added back and defined on `sqlgen.Schema`. ([#195](https://github.com/samsarahq/thunder/pull/195))
- `UnbuildStruct` is now defined `sqlgen.Schema`. It's not a package level
  function anymore. ([#195](https://github.com/samsarahq/thunder/pull/195))

## [0.4.0] - 2018-09-13

### Changed

- Memory optimizations (see [#166](https://github.com/samsarahq/thunder/pull/166))

### Removed

#### `fields`

- This is now internal API and will no longer be documented in the changelog.

#### `sqlgen`

- `BuildStruct` is no longer necessary and was removed.

## [0.3.1] - 2018-08-28

### Changed

#### `sqlgen`

- Handle MySQL `time.Time` types by converting `time.Time` using
  `github.com/go-sql-driver/mysql.NullTime`. `DATE`/`DATETIME` are returned as
  formatted strings.

## [0.3.0] - 2018-08-27

### Changed

#### `sqlgen`

- Pointer scanners are no longer allowed to handle `nil`, are forcefully set to
  `nil` instead.

### Removed

#### `fields`

- Removed `Scanner.Interface()`: this should never have been exposed and
  wouldn't work the way you would expect it to. Instead, you can only copy to an
  existing `reflect.Value`.


## [0.2.0] - 2018-08-23

### Added

#### `sqlgen`, `livesql`

- Using `sql:",json"` on a struct field will (de)serialize it into JSON
  using `json.Marshal`/`json.Unmarshal`.
- Using `sql:",binary"` on a struct field will attempt to (de)serialize it
  into binary using `encoding.BinaryMarshaler`/`encoding.BinaryUnmarshaler` -
  also respects `Marshal`/`Unmarshal` methods (works with gogo/protobuf)
- Using `sql:",string"` on a struct field will attempt to (de)serialize it
  into a string using `encoding.TextMarshaler`/`encoding.TextUnmarshaler`
- Respect `sql.Scanner` and `driver.Valuer` interfaces the same way the sql
  package would.

### Changed

#### `sqlgen`

- Automatic inference of sql types from struct field types:
  - all sub-types of bool (eg: `type foo bool`) are coerced into `bool`.
  - all sub-types of string (eg: `type foo string`) are coerced into `string`.
  - `int`/`int8`/`int16`/`int32`/`int64`/`uint`/`uint8`/`uint16`/`uint32`/`uint64`
    and all sub-types (eg: `type foo int16`) are coerced into `int64`.
  - `float32`/`float64` and all sub-types (eg: `type foo float32`) are coerced
    into `float64`.

### Removed

#### `sqlgen`

- Removed manual registration of types:
  - `MustRegisterCustomScalar`
  - `RegisterCustomScalar`
  - `MustRegisterSimpleScalar`
  - `RegisterSimpleScalar`

## [0.1.0] - 2018-08-23

First entry
