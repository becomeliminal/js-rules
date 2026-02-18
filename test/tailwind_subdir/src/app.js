// This file contains Tailwind utility class names that Tailwind must scan.
// The tailwind.config.js uses content: ["./src/**/*.js"] — a relative path
// that only resolves correctly if Tailwind runs with CWD set to the config dir.

export function render() {
    return '<div class="bg-blue-500 text-white p-4">Hello from subdir</div>';
}
