import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// A single-page app: the Go server embeds the build and serves
			// index.html for every route it does not recognise.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		// ws: the draft room's live connection goes through the same proxy.
		proxy: { '/api': { target: 'http://localhost:8090', ws: true } }
	}
});
