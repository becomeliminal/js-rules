import { EventEmitter } from "events";
const ee = new EventEmitter();
ee.on("test", () => console.log("ok"));
ee.emit("test");
