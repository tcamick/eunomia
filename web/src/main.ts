import { mount } from 'svelte';
import '@fontsource-variable/public-sans';
import './app.css';
import App from './App.svelte';
import { router } from './lib/router.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app');
router.start();
mount(App, { target });
