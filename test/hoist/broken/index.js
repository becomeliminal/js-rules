// Imports ms without declaring it -- standing in for the third-party package
// with a missing manifest entry that public hoisting exists to work around.
const ms = require("ms");

exports.oneMinute = () => ms(60000);
