# Changelog

## 0.2.1
- Fix a bug where subsciptions may incorrectly toggle when props change before an initial load. ([#119)(https://github.com/samsarahq/thunder/pull/119))
- Remove unused references to requestId

## 0.2.0
- Add support for automatic type introspection with [`webpack-graphql-loader`](https://github.com/samsarahq/graphql-loader) ([#108](https://github.com/samsarahq/thunder/pull/108))

## 0.1.1
- Fix missing type exports.
- Bake in transform runtime so that `regeneratorRuntime` is not required by userspace code.

## 0.1.0
- Initial typescript release.
