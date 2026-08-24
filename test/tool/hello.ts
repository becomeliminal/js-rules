interface Greeting {
    name: string;
    times: number;
}

export function greet({ name, times }: Greeting): string {
    return Array.from({ length: times }, () => `hello ${name}`).join(", ");
}

console.log(greet({ name: "please", times: 2 }));
