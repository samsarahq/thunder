# Changelog

## [Unreleased]

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
