const {test, expect} = require('@playwright/test');

// Perf bench: each test seeds a heavy fixture, opens it via the same code
// path a real user hits, and prints `<label>: <median> ms (samples: …)`.
//
// Run locally only - timings vary 2-3x across machines, so absolute
// thresholds aren't useful in CI. The numbers go to stdout; compare a
// run before/after a change to spot regressions.
//
//   make e2es test="perf"
//   make e2esh test="perf"   # headed, to watch

const RUNS = 5;

function median(samples) {
    const sorted = [...samples].sort((a, b) => a - b);
    const mid = Math.floor(sorted.length / 2);
    return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

test.beforeEach(async ({page}) => {
    await page.goto('/index.html');
    await page.waitForSelector('#tree', {timeout: 1000});
});

test('open 1k-line plain markdown file', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fh = await root.getFileHandle('Big.md', {create: true});
            const w = await fh.createWritable();
            let buf = '';
            for (let i = 0; i < 1000; i++) {
                buf += `Line ${i} with prose, **bold**, *italic*, and \`code\`.\n`;
            }
            await w.write(buf);
            await w.close();
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById("editor")));
    await page.waitForTimeout(300);

    const samples = [];
    for (let i = 0; i < RUNS; i++) {
        const ms = await page.evaluate(async () => {
            const t = performance.now();
            await openFile('/Big.md');
            return performance.now() - t;
        });
        samples.push(ms);
        // Bounce to another file so the next open isn't a no-op same-file path.
        await page.evaluate(async () => {
            await openFile('/🪴 Welcome.md').catch(() => {});
        });
        await page.waitForTimeout(50);
    }

    console.log(`Big.md: ${median(samples).toFixed(1)} ms (samples: ${samples.map(s => s.toFixed(0)).join(', ')})`);
});

test('open file with 50 mermaid blocks', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fh = await root.getFileHandle('Diagrams.md', {create: true});
            const w = await fh.createWritable();
            let buf = '';
            for (let i = 0; i < 50; i++) {
                buf += '```mermaid\nflowchart LR\n';
                buf += `    A${i}[node ${i}] --> B${i}[next]\n`;
                buf += `    B${i} --> C${i}[end]\n`;
                buf += '```\n\n';
            }
            await w.write(buf);
            await w.close();
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById("editor")));
    await page.waitForTimeout(300);

    const samples = [];
    for (let i = 0; i < RUNS; i++) {
        const ms = await page.evaluate(async () => {
            const t = performance.now();
            await openFile('/Diagrams.md');
            return performance.now() - t;
        });
        samples.push(ms);
        await page.evaluate(async () => {
            await openFile('/🪴 Welcome.md').catch(() => {});
        });
        await page.waitForTimeout(50);
    }

    console.log(`Diagrams.md: ${median(samples).toFixed(1)} ms (samples: ${samples.map(s => s.toFixed(0)).join(', ')})`);
});

test('open file with 200 LaTeX blocks', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fh = await root.getFileHandle('Math.md', {create: true});
            const w = await fh.createWritable();
            let buf = '';
            for (let i = 0; i < 200; i++) {
                buf += `Inline math: $F_${i} = m a_${i}$ and another $\\frac{a}{b}$\n\n`;
                buf += `$$\\int_0^${i} e^x \\,dx = e^${i} - 1$$\n\n`;
            }
            await w.write(buf);
            await w.close();
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById("editor")));
    await page.waitForTimeout(300);

    const samples = [];
    for (let i = 0; i < RUNS; i++) {
        const ms = await page.evaluate(async () => {
            const t = performance.now();
            await openFile('/Math.md');
            return performance.now() - t;
        });
        samples.push(ms);
        await page.evaluate(async () => {
            await openFile('/🪴 Welcome.md').catch(() => {});
        });
        await page.waitForTimeout(50);
    }

    console.log(`Math.md: ${median(samples).toFixed(1)} ms (samples: ${samples.map(s => s.toFixed(0)).join(', ')})`);
});

test('sidebar render with 1000 files in one folder', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const dir = await root.getDirectoryHandle('many', {create: true});
            for (let i = 0; i < 1000; i++) {
                const fh = await dir.getFileHandle(`note-${i}.md`, {create: true});
                const w = await fh.createWritable();
                await w.write(`# note ${i}\nbody`);
                await w.close();
            }
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById("editor")));
    await page.waitForTimeout(500);

    // Trigger a fresh renderSidebar() and time it. selectSidebarItem walks
    // the existing tree; renderSidebar rebuilds it from `files`.
    const samples = [];
    for (let i = 0; i < RUNS; i++) {
        const ms = await page.evaluate(() => {
            const t = performance.now();
            renderSidebar();
            return performance.now() - t;
        });
        samples.push(ms);
    }

    console.log(`renderSidebar (1000 files): ${median(samples).toFixed(1)} ms (samples: ${samples.map(s => s.toFixed(0)).join(', ')})`);
});
