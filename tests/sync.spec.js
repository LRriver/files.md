const {test, expect} = require('@playwright/test');
const fs = require('fs').promises;
const path = require('path');
const crypto = require('crypto');

test.beforeEach(async ({page}) => {
    const serverDir = '../storage/-1';
    await clearDirectory(serverDir);

    const filePath = path.join(serverDir, saltToken('token'));
    try {
        await fs.writeFile(filePath, '-1', 'utf8');
    } catch (error) {
        console.error('Error creating file:', error);
    }

    await page.addInitScript(() => {
        window.API_HOST = 'http://localhost:8080';
        localStorage.setItem('token', 'token');

    });

    await page.goto('/app.html');

    await page.evaluate(()=> {
        window.getRootDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subdir = await root.getDirectoryHandle('subdir', { create: true });

            const files = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const file of files) {
                try {
                    await subdir.getFileHandle(file.name);
                } catch (error) {
                    const fileHandle = await subdir.getFileHandle(file.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(file.content);
                    await writable.close();
                }
            }

            return root;
        };
    })
    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });

    await page.waitForSelector('.CodeMirror', {timeout: 10000});
    await page.waitForSelector('#sidebar-tree', {timeout: 5000});
});

test('sync', async ({ page }) => {
    await page.pause();
});

async function clearDirectory(dirPath) {
    try {
        const items = await fs.readdir(dirPath);

        for (const item of items) {
            const itemPath = path.join(dirPath, item);
            const stat = await fs.stat(itemPath);

            if (stat.isDirectory()) {
                // Recursively delete subdirectory
                await fs.rm(itemPath, { recursive: true, force: true });
            } else {
                // Delete file
                await fs.unlink(itemPath);
            }
        }

        console.log(`Cleared directory: ${dirPath}`);
    } catch (error) {
        console.log(`Error clearing directory ${dirPath}:`, error.message);
    }
}

function saltToken(token, salt = "") {
    return crypto.createHash('sha256')
        .update(token + salt)
        .digest('hex');
}