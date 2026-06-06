const {test, expect} = require('@playwright/test');

test.beforeEach(async ({page}) => {
    test.setTimeout(15000);
    await page.goto('/index.html');

    await page.waitForSelector('#tree', {timeout: 5000});
});

async function writeRootFile(page, path, content) {
    await page.evaluate(async ({path, content}) => {
        const root = await navigator.storage.getDirectory();
        const parts = path.replace(/^\/+/, '').split('/');
        const filename = parts.pop();
        let dir = root;
        for (const part of parts) {
            dir = await dir.getDirectoryHandle(part, {create: true});
        }
        const fh = await dir.getFileHandle(filename, {create: true});
        const w = await fh.createWritable();
        await w.write(content);
        await w.close();
    }, {path, content});
}

async function useExplicitTestApi(page) {
    await page.evaluate(() => {
        localStorage.setItem('apiUrl', window.location.origin);
        localStorage.removeItem('lastServerOk');
    });
}

async function reloadApp(page) {
    await page.reload();
    await page.waitForSelector('#tree', {timeout: 5000});
}

async function waitForAssistantAvailable(page) {
    await expect(page.locator('[data-testid="assistant-panel"]')).toHaveAttribute('data-available', 'true');
}

async function routeAssistantStatus(page, response) {
    const requests = [];
    await page.route('**/llmStatus', async route => {
        requests.push(route.request());
        await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(response),
        });
    });
    return requests;
}

async function routeAssistantChat(page, handler) {
    const requests = [];
    await page.route('**/llmChat', async route => {
        const request = route.request();
        requests.push(request);
        const payload = request.postDataJSON();
        const response = await handler(payload, request);
        await route.fulfill({
            status: response.status || 200,
            contentType: 'application/json',
            body: JSON.stringify(response.body || {
                status: 'ok',
                requestId: 'test-request',
                model: 'test-model',
                text: 'Draft from test assistant',
            }),
        });
    });
    return requests;
}

test('send message to chat', async ({ page }) => {
    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    // TODO I believe chat is reloaded 2 times for some reason, it blinks, and thus removes previous message
    // Or wait for timeout before typing message doesn't help hmm
    await page.keyboard.press('Enter');

    await page.waitForSelector('.message');
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

});

test('does not probe llm status on default hosted api without linked server evidence', async ({page}) => {
    await page.evaluate(() => {
        localStorage.removeItem('apiUrl');
        localStorage.removeItem('lastServerOk');
    });
    const statusRequests = await routeAssistantStatus(page, {
        status: 'ok',
        available: true,
        model: 'should-not-be-probed',
    });

    await reloadApp(page);

    await expect(page.locator('[data-testid="assistant-panel"]')).toHaveAttribute('data-available', 'false');
    await expect(page.locator('[data-testid="assistant-action-selected"]')).toBeDisabled();
    await page.waitForTimeout(300);
    expect(statusRequests).toHaveLength(0);
});

test('ordinary chat capture never calls llmChat', async ({page}) => {
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const chatRequests = await routeAssistantChat(page, async () => ({
        body: {status: 'ok', requestId: 'unexpected', text: 'Should not happen'},
    }));
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await page.waitForSelector('#chat');
    await page.locator('#chat-input').click();
    await page.keyboard.type('Capture only');
    await page.keyboard.press('Enter');

    await expect(page.locator('.message-content').last()).toHaveText('Capture only');
    expect(chatRequests).toHaveLength(0);
});

test('chat input switches between microphone and send controls', async ({page}) => {
    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');

    await expect(page.locator('#mic-chat')).toBeVisible();
    await expect(page.locator('#send-chat')).toBeHidden();

    await page.locator('#chat-input').fill('Ready to send');
    await expect(page.locator('#send-chat')).toBeVisible();
    await expect(page.locator('#mic-chat')).toBeHidden();

    await page.locator('#chat-input').fill('');
    await expect(page.locator('#mic-chat')).toBeVisible();
    await expect(page.locator('#send-chat')).toBeHidden();
});

test('journal suffix capture does not call llmChat', async ({page}) => {
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const chatRequests = await routeAssistantChat(page, async () => ({
        body: {status: 'ok', requestId: 'unexpected', text: 'Should not happen'},
    }));
    await reloadApp(page);

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.locator('#chat-input').fill('Journal capture jj');
    await page.keyboard.press('Enter');

    await expect(page.locator('#tree')).toContainText('journal');
    await expect(page.locator('#chat-input')).toHaveValue('');
    const chatMessages = await page.locator('.message-content').allTextContents();
    expect(chatMessages.join('\n')).not.toContain('Journal capture');
    expect(chatRequests).toHaveLength(0);
});

test('pasted image capture inserts markdown and does not call llmChat', async ({page}) => {
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const chatRequests = await routeAssistantChat(page, async () => ({
        body: {status: 'ok', requestId: 'unexpected', text: 'Should not happen'},
    }));
    await reloadApp(page);

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.locator('#chat-input').click();
    await page.evaluate(() => {
        const file = new File(['fake image'], 'clip.png', {type: 'image/png'});
        const data = new DataTransfer();
        data.items.add(file);
        document.getElementById('chat-input').dispatchEvent(new ClipboardEvent('paste', {
            clipboardData: data,
            bubbles: true,
            cancelable: true,
        }));
    });

    await expect(page.locator('#chat-input')).toHaveValue(/!\[[^\]]+\.png\]\(media\/[^)]*\.png\)\n/);
    expect(chatRequests).toHaveLength(0);
});

test('voice capture appends media markdown and does not call llmChat', async ({page}) => {
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const chatRequests = await routeAssistantChat(page, async () => ({
        body: {status: 'ok', requestId: 'unexpected', text: 'Should not happen'},
    }));
    await reloadApp(page);
    await page.evaluate(() => {
        Object.defineProperty(navigator, 'mediaDevices', {
            configurable: true,
            value: {
                getUserMedia: async () => ({
                    getTracks: () => [{stop() {}}],
                }),
            },
        });
        window.MediaRecorder = class FakeMediaRecorder {
            static isTypeSupported() { return true; }
            constructor() {
                this.state = 'inactive';
                this.mimeType = 'audio/webm';
            }
            start() {
                this.state = 'recording';
            }
            stop() {
                this.state = 'inactive';
                this.ondataavailable?.({data: new Blob(['voice'], {type: 'audio/webm'})});
                this.onstop?.();
            }
        };
    });

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.locator('#mic-chat').click();
    await expect(page.locator('#mic-chat')).toHaveClass(/recording/);
    await page.locator('#mic-chat').click();

    await expect(page.locator('.message-content').last()).toHaveAttribute('data-text', /!\[\]\(media\/.*\.weba\)/);
    expect(chatRequests).toHaveLength(0);
});

test('selected-chat assistant sends only selected texts and no unrelated app data', async ({page}) => {
    await writeRootFile(page, '/Chat.md', [
        '#### 4 June, Thursday',
        '- [ ] `09:00` Selected item',
        '- [ ] `09:01` Other item',
        '',
    ].join('\n'));
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const payloads = [];
    await routeAssistantChat(page, async payload => {
        payloads.push(payload);
        return {body: {status: 'ok', requestId: 'selected-1', model: 'test-model', text: 'Only first item'}};
    });
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await expect(page.locator('.message-content')).toHaveCount(2);

    await page.locator('.message[data-text="Selected item"]').evaluate(el => {
        document.querySelectorAll('.message.selected').forEach(message => message.classList.remove('selected'));
        el.classList.add('selected');
        updateAssistantPanel();
    });
    await expect(page.getByTestId('assistant-action-selected')).toBeEnabled();
    await page.getByTestId('assistant-action-selected').click();
    await page.getByTestId('assistant-send').click();

    await expect(page.getByTestId('assistant-draft')).toContainText('Only first item');
    expect(payloads).toHaveLength(1);
    expect(payloads[0].action).toBe('summarize');
    expect(payloads[0].contexts).toHaveLength(1);
    expect(payloads[0].contexts[0]).toMatchObject({
        source: 'selected-chat',
        label: 'Selected chat entries',
    });
    expect(payloads[0].contexts[0].text).toContain('Selected item');
    expect(payloads[0].contexts[0].text).not.toContain('Other item');
    expect(JSON.stringify(payloads[0])).not.toContain('syncFilenames');
    expect(JSON.stringify(payloads[0])).not.toContain('serverTime');
    expect(JSON.stringify(payloads[0])).not.toContain('WELCOME_FILES');
});

test('current-file assistant requires confirmation and sends the labeled file only', async ({page}) => {
    test.setTimeout(15000);
    await writeRootFile(page, '/Notes.md', 'Important note body');
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    const payloads = [];
    await routeAssistantChat(page, async payload => {
        payloads.push(payload);
        return {body: {status: 'ok', requestId: 'file-1', model: 'test-model', text: 'File draft'}};
    });
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await page.evaluate(() => openFile('/Notes.md'));
    await page.evaluate(() => openChatModal());
    await expect(page.getByTestId('assistant-action-current-file')).toBeEnabled();
    await page.getByTestId('assistant-action-current-file').click({force: true});

    await expect(page.getByTestId('assistant-context-label')).toContainText('/Notes.md');
    await expect(page.getByTestId('assistant-send')).toBeDisabled();

    await page.getByTestId('assistant-confirm-context').check();
    await page.getByTestId('assistant-send').click();

    await expect(page.getByTestId('assistant-draft')).toContainText('File draft');
    expect(payloads).toHaveLength(1);
    expect(payloads[0].contexts).toHaveLength(1);
    expect(payloads[0].contexts[0]).toMatchObject({
        source: 'current-file',
        label: 'Current file: /Notes.md',
        path: '/Notes.md',
    });
    expect(payloads[0].contexts[0].text).toContain('Important note body');
    expect(JSON.stringify(payloads[0])).not.toContain('Selected item');
    expect(JSON.stringify(payloads[0])).not.toContain('syncFilenames');
});

test('fullscreen Chat.md disables current-file assistant context', async ({page}) => {
    test.setTimeout(15000);
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await expect(page.getByTestId('assistant-action-current-file')).toBeDisabled();
    await expect(page.getByTestId('assistant-context-label')).toContainText('Select chat entries or open a note');
});

test('current-file assistant context clears when switching to fullscreen Chat.md', async ({page}) => {
    test.setTimeout(15000);
    await writeRootFile(page, '/Notes.md', 'Important note body');
    await writeRootFile(page, '/Chat.md', '#### 4 June, Thursday\n- [ ] `09:00` Selected item\n');
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.evaluate(() => {
        llmAssistantState.contexts = [{
            source: 'current-file',
            label: 'Current file: /Notes.md',
            path: '/Notes.md',
            text: 'Important note body',
        }];
        llmAssistantState.confirmed = true;
        updateAssistantPanel();
    });
    await expect(page.getByTestId('assistant-context-label')).toContainText('/Notes.md');

    await page.evaluate(async () => openChat());

    await expect(page.getByTestId('assistant-action-current-file')).toBeDisabled();
    await expect(page.getByTestId('assistant-context-label')).toContainText('Select chat entries or open a note');
    await expect(page.getByTestId('assistant-confirm-context')).not.toBeVisible();
});

test('assistant draft can append to Chat.md and insert into the current file', async ({page}) => {
    test.setTimeout(15000);
    await writeRootFile(page, '/Notes.md', 'Original note');
    await useExplicitTestApi(page);
    await routeAssistantStatus(page, {status: 'ok', available: true, model: 'test-model'});
    await routeAssistantChat(page, async () => ({
        body: {
            status: 'ok',
            requestId: 'draft-1',
            model: 'test-model',
            text: 'Heading\n- [ ] checklist-looking\n> quote',
        },
    }));
    await reloadApp(page);
    await waitForAssistantAvailable(page);

    await page.evaluate(() => openFile('/Notes.md'));
    await page.evaluate(() => openChatModal());
    await expect(page.getByTestId('assistant-action-current-file')).toBeEnabled();
    await page.getByTestId('assistant-action-current-file').click({force: true});
    await page.getByTestId('assistant-confirm-context').check();
    await page.getByTestId('assistant-send').click();
    await expect(page.getByTestId('assistant-draft')).toContainText('Heading');

    await page.getByTestId('assistant-append-chat').click();
    await page.click(`#tree .tree-item:has-text('chat')`);
    await expect(page.locator('.message-content').last()).toContainText('AI: Heading');
    await expect(page.locator('.message-content').last()).toContainText('- [ ] checklist-looking');

    await page.evaluate(() => openFile('/Notes.md'));
    await page.evaluate(() => openChatModal());
    await expect(page.getByTestId('assistant-action-current-file')).toBeEnabled();
    await page.getByTestId('assistant-action-current-file').click({force: true});
    await page.getByTestId('assistant-confirm-context').check();
    await page.getByTestId('assistant-send').click();
    await page.getByTestId('assistant-insert-current').click();

    const content = await page.evaluate(() => document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(content).toContain('Original note');
    expect(content).toContain('Heading');
});

test('select all in chat input selects input text, not bubbles', async ({page}) => {
    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('First message');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.locator('#chat-input').click();
    await page.keyboard.type('to be cleared');
    await expect(page.locator('#chat-input')).toHaveValue('to be cleared');

    const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press('Delete');

    await expect(page.locator('#chat-input')).toHaveValue('');
    await expect(page.locator('.message-content')).toHaveText('First message');
});

test('move to dir creates a new file inside that dir', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getDirectoryHandle('projects', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('MyTask');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-dir="projects"]').evaluate(el => el.click());
    await page.waitForSelector('.message', {state: 'detached'});

    const exists = await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const projects = await root.getDirectoryHandle('projects');
        try { await projects.getFileHandle('MyTask.md'); return true; }
        catch { return false; }
    });
    expect(exists).toBe(true);
});

test('move to root creates a new file at root', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            return await navigator.storage.getDirectory();
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('RootMsg');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-dir=""]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    const exists = await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        try { await root.getFileHandle('RootMsg.md'); return true; }
        catch { return false; }
    });
    expect(exists).toBe(true);
});

test('move to existing file appends content', async ({page}) => {
    // Seed once, then return the same root on subsequent calls — otherwise the
    // app's repeated getRootDirHandle() calls would re-overwrite Notes.md.
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Append me');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() => document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(content).toContain('# Notes');
    expect(content).toContain('Append me');
});

test('move to file does not prepend a timestamp', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Attention is all you need');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() => document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(content).toContain('Attention is all you need');
    // The body must not be prefixed with `HH:MM` - that's reserved for the
    // chat→journal flow, not move-to-file (web/lib/md.js:addHeaderAndText).
    expect(content).not.toMatch(/`\d{2}:\d{2}`\s*500k/);
});

test('move to recent file does not prepend a timestamp', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getFileHandle('File.md', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Attention is all you need');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-recent-btn[data-filename="File.md"]').click({force: true});
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    const fileContent = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(fileContent).toContain('Attention is all you need');
    expect(fileContent).not.toMatch(/`\d{2}:\d{2}`\s*500k/);
});

test('system dirs (archive, today) are hidden in move-to-file modal', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getDirectoryHandle('archive', {create: true});
            await root.getDirectoryHandle('projects', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Hello');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await expect(page.locator('#search-results li[data-dir="projects"]')).toBeVisible();
    await expect(page.locator('#search-results li[data-dir="archive"]')).toHaveCount(0);
    await expect(page.locator('#search-results li[data-dir="today"]')).toHaveCount(0);
});

test('send to chat and move to recent file', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fileHandle = await root.getFileHandle('File.md', { create: true });

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');

    await page.waitForSelector('.message');
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

    await page.hover('.message');
    await page.locator('.to-recent-btn[data-filename="File.md"]').click({force: true});
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    const fileContent = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(fileContent).toContain('# File');
    expect(fileContent).toContain('My message');
});

// Regression: moving a lowercase-starting chat message via the to-file-btn
// used to crash because chat.js applied ucfirst() to the text before passing
// it to the search modal, while the DOM dataset.text stayed in original case.
// `find(el => el.dataset.text === selectedMsgText)` then returned undefined
// and modals.js threw "Cannot read properties of undefined (reading 'classList')".
test('move-to-file works for messages that start with a lowercase letter', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    // Lowercase starting letter is the trigger for the original bug.
    await page.keyboard.type('lowercase start');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    const errors = [];
    page.on('pageerror', err => errors.push(err.message));

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    // No uncaught exceptions should have been raised during the flow.
    expect(errors).toEqual([]);

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    // The text written to the file is capitalised (ucfirst applied at the
    // write step, AFTER the DOM lookup, so it doesn't break find()).
    expect(content).toContain('Lowercase start');
});

// Regression: a chat message containing `"` used to crash to-file because
// escapeHtml() left quotes unescaped, the `data-text="..."` attribute closed
// early at the first `"`, and the modal's `dataset.text === selectedMsgText`
// lookup returned undefined.
test('move-to-file works for messages containing double quotes', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    const quoted = 'catches "file changed in vim." Without it';
    await page.keyboard.type(quoted);
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    const errors = [];
    page.on('pageerror', err => errors.push(err.message));

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    expect(errors).toEqual([]);

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    // ucfirst capitalises the first letter at the write step (see the
    // lowercase-letter regression test above). What matters here is that
    // the embedded quotes round-trip intact.
    expect(content).toContain('Catches "file changed in vim." Without it');
});

// Regression: clicking the complete checkbox on a message containing `"` used
// to be a no-op - escapeHtml() left the quote unescaped in
// `data-text="..."`, so el.dataset.text truncated at the first `"`, the
// regex in toggleChatMessage didn't match anything, and the on-disk `- [ ]`
// stayed `- [ ]`.
test('complete-btn toggles a message containing double quotes', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    const quoted = 'ask about "uuid": "019e4eea-32b1-7c08-a000-3a1ecd0a6c07"';
    await page.keyboard.type(quoted);
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    const errors = [];
    page.on('pageerror', err => errors.push(err.message));

    await page.hover('.message');
    await page.locator('.message .complete-btn').first().click({force: true});
    await expect(page.locator('.message.completed')).toHaveCount(1);

    // Reread Chat.md from disk - the toggle should have rewritten the line
    // from `- [ ]` to `- [x]`. Before the fix, the regex match in
    // toggleChatMessage failed (dataset.text was truncated at the first `"`)
    // and the file was left untouched.
    await expect.poll(async () => {
        return await page.evaluate(async () => {
            const root = await navigator.storage.getDirectory();
            const fh = await root.getFileHandle('Chat.md');
            return (await fh.getFile()).text();
        });
    }).toMatch(/^- \[x\] `\d{2}:\d{2}` ask about "uuid": "019e4eea-32b1-7c08-a000-3a1ecd0a6c07"\s*$/m);
    expect(errors).toEqual([]);
});
