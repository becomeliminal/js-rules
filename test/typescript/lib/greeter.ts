export interface GreetOptions {
  name: string;
  greeting?: string;
}

export function greet(opts: GreetOptions): string {
  const greeting = opts.greeting || "Hello";
  return `${greeting}, ${opts.name}!`;
}

export function greetAll(names: string[]): string[] {
  return names.map(name => greet({ name }));
}
