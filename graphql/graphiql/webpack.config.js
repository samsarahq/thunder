const HtmlWebpackPlugin = require('html-webpack-plugin');
const webpack = require('webpack');
const path = require('path');

module.exports = {
  entry: './index.jsx',
  output: {
    path: path.resolve(__dirname, 'dist'),
    filename: 'bundle.js'
  },
  module: {
    rules: [
      { test: /\.(js|jsx)$/, exclude: /(node_modules|bower_components)/, loader: 'babel-loader', query: { presets: ['es2015', 'react'], plugins: ['transform-object-rest-spread'] } },
      { test: /\.css$/, use: [ { loader: 'style-loader' }, { loader: 'css-loader' } ] }
    ]
  },
  plugins: [
    new HtmlWebpackPlugin({template: './index.html'})
  ]
};

