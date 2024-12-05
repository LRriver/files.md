let editor = null;
let searchList = [];
let focusedItemIndex = -1;

function initHyperMD(el) {
    editor = HyperMD.fromTextArea(el, {
        mode: "hypermd",
        lineNumbers: false,
        extraKeys: {
            // "Shift-Space": "autocomplete",
            'Cmd-[': false,
            'Cmd-]': false,
        },
        hintOptions: {
            hint: CompleteEmoji.createHintFunc(),
            closeCharacters: /$^/,
            closeOnUnfocus: false,
            completeSingle: false,
            alignWithWord: false
        },
        hmdFoldEmoji: {
            myEmoji: createAutocompleteDict
        }
    });
    editor.setSize(null, "100%");

    editor.addKeyMap({
        'Cmd-Y': function(cm) {
            cm.replaceSelection('✅');
            cm.focus();
        }
    });

    editor.hmdResolveURL = function (path) {
        if (typeof path === 'undefined') {
            return path
        }

        path = path.replace(/%20/g, " ");

        if (/^(?!http|https|\[).+\.md$/.test(path)) {
            let parts = path.split('/');
            if (parts.length === 1) {
                showFile("", path);
                return;
            }
            showFile(parts[0], parts[1]);
            return path;
        }

        const match = path.match(/^img\/(.+\.(png|jpg|jpeg|gif))$/i);

        if (match && zettel['img'] && zettel['img'][match[1]]) {
            return zettel['img'][match[1]].url;
        }

        return path;
    };
    editor.hmdReadLink = async function (path) {
        path = path.replace('[', '').replace(']', '');
        let parts = path.split('/');
        if (parts.length === 1) {
            await showFile("", path + '.md');
            return;
        }

        await showFile(parts[0], parts[1] + '.md');
    };
    editor.on("inputRead", async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            editor.showHint({
                completeSingle: false,
                updateOnCursorActivity: true,
            })
        }
    })

    editor.on("change", async function (cm, changeObj) {
        // Save on user input only
        if (changeObj.origin && changeObj.origin !== "setValue") {
            await saveFile();
        }
    });
    editor.setOption("viewportMargin", Infinity);
    editor.on("paste", async (_, event) => {
        const items = (event.clipboardData || event.originalEvent.clipboardData).items;
        for (const item of items) {
            if (item.kind === "file" && item.type.startsWith("image/")) {
                const file = item.getAsFile();
                const fileName = `${new Date().toISOString().replace(/[:.]/g, '-')}.png`;
                await saveImageToDirectory(file, fileName);

                const markdownImageSyntax = `![](img/${fileName})`;
                editor.replaceSelection(markdownImageSyntax);
                // if (fileHandle) {
                //     // Insert the Markdown image syntax into the editor
                //     const markdownImageSyntax = `![[${fileName}]]`;
                //     editor.replaceSelection(markdownImageSyntax);
                //     console.log(`Image saved as: ${fileName}`);
                // } else {
                //     console.error("Failed to save the image.");
                // }
            }
        }
    });
    editor.addKeyMap({
        // For some reason in default configuration cursor can't be moved up if we have an image above
        "Up": function(cm) {
            const cursor = cm.getCursor();
            let lineAbove = cursor.line - 1;
            lineAbove = Math.max(0, lineAbove);

            const aboveLineContent = cm.getLine(lineAbove);
            let imgRE = /!\[.*?\]\(.*?\)/
            const hasImageAbove = imgRE.test(aboveLineContent) || imgRE.test(cm.getLine(cursor.line));
            if (hasImageAbove) {
                cm.setCursor(lineAbove, 0);
            } else {
                cm.execCommand("goLineUp");
            }
        }
    });
}

// We use it as links autocomplete
function createAutocompleteDict() {
    const emojiDict = {};
    const ignoredFolders = ["img", "archive", "_read_", "_watch_", "_shop_", "today", "later", "journal", "habits", "triggers", "places", ""];

    Object.keys(zettel).forEach(folder => {
        if (ignoredFolders.includes(folder)) return;

        Object.keys(zettel[folder]).forEach(filename => {
            const emojiKey = `${filename.replace(/\.md$/, "").replace(/\.[^/.]+$/, "")}`;
            const filePath = `${filename.replace(/\.md$/, "")}](${folder}/${filename})`;
            emojiDict[emojiKey] = filePath;
        });
    });

    return emojiDict;
}

async function saveImageToDirectory(file, fileName) {
    try {
        let imgDirHandle = await getSavedDirectoryHandle();
        if (!imgDirHandle) return null;

        // Create a file handle for the image file
        const fileHandle = await imgDirHandle.getFileHandle(fileName, {create: true});
        const writable = await fileHandle.createWritable();
        await writable.write(file);
        await writable.close();

        console.log("Image saved");
        return fileHandle;
    } catch (error) {
        console.error("Error saving image:", error);
        return null;
    }
}

async function getImageUrl(fileHandle) {
    const file = await fileHandle.getFile();
    return URL.createObjectURL(file);
}

function buildSidebar() {
    let root = new TreeNode("zettel"); // Create the root-node
    for (dir in zettel) {
        if (dir === '' || dir === 'img') {
            continue;
        }

        let dirNode = new TreeNode(dir, {expanded: false});
        for (let file in zettel[dir]) {
            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await showFile(node.parent.toString(), node.toString() + ".md");
            });
            dirNode.addChild(fileNode);
        }
        root.addChild(dirNode);
    }

    if (zettel['']) {
        // Adding root files after folders
        for (let file in zettel[""]) {
            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await showFile("", node.toString() + ".md");
            });
            root.addChild(fileNode)
        }
    }

    new TreeView(root, "#editor-sidebar", {
        show_root: false,
    });
}

async function openDirectory() {
    zettel = {};
    let dirHandle = await window.showDirectoryPicker();
    document.getElementById('welcome').style.display = 'none';
    await saveDirectoryHandle(dirHandle);
    await loadDirectory(dirHandle)
    await showRandomFile();
}

async function loadDirectory(dirHandle, path = "", depth = 1) {
    const entries = [];
    for await (const entry of dirHandle.values()) {
        entries.push(entry);
    }
    entries.sort((a, b) => a.name.localeCompare(b.name));

    for await (const entry of entries) {
        const filename = entry.name.normalize("NFC");
        if (entry.kind === 'directory') {
            if (filename.startsWith('.')) continue;

            if (depth < 2) {
                const folder = `${path}${filename}/`;
                zettel[filename] = {};
                await loadDirectory(entry, folder, depth + 1);
            }
        } else if (entry.kind === 'file' && filename.endsWith('.png') ||
            filename.endsWith('.jpg') ||
            filename.endsWith('.jpeg') ||
            filename.endsWith('.gif') ||
            filename.endsWith('.md') ||
            filename.endsWith('.txt')
        ) {
            const folder = path.split('/').filter(Boolean).join('/');
            if (!zettel[folder]) zettel[folder] = {};
            if (folder === 'img') {
                zettel[folder][filename] = {handle: entry, url: await getImageUrl(entry)};
            } else {
                zettel[folder][filename] = {handle: entry};
            }
        }
    }
    // Remove empty folders
    for (const folder in zettel) {
        if (Object.keys(zettel[folder]).length === 0) {
            delete zettel[folder];
        }
    }
    buildSidebar();
}

async function showRandomFile() {
    const ignoredFolders = ["img", "archive", "_read_", "_watch_", "_shop_", "today", "later", "journal", "habits", "triggers", "places", ""];

    const allFiles = [];
    for (let folder in zettel) {
        if (ignoredFolders.includes(folder)) continue;

        for (let file in zettel[folder]) {
            allFiles.push({folder, file});
        }
    }

    if (allFiles.length === 0) {
        console.error("No files found to open.");
        return;
    }

    const randomFile = allFiles[Math.floor(Math.random() * allFiles.length)];

    try {
        await showFile(randomFile.folder, randomFile.file);
    } catch (error) {
        console.error("Failed to open random file:", error);
    }
}

async function showFile(folder, filename, saveToHistory = true) {
    filename = filename.normalize("NFC");
    const fileData = zettel[folder][filename];
    const file = await fileData.handle.getFile();
    const header = filename.replace(/\.md$/, "").replace(/^\w/, (c) => c.toUpperCase());
    let content = await file.text();
    content = `# ${header}\n${content}`;
    content = content.replace(/\[\[(.+?)\|.*?\]\]/g, '[[$1]]');

    editor.currentFolder = folder;
    editor.currentFile = filename;
    if (saveToHistory) {
        const state = {folder: folder, file: filename};
        history.pushState(state, '');
    }

    editor.getDoc().setValue(content);
    editor.clearHistory();

    // Set cursor at the end of the page.
    // We need to execute this code after some rendering loop. If we don't do that,
    // Images and other heavy stuff won't be loaded
    setTimeout(() => {
        const lastLine = editor.lastLine();
        let targetLine = lastLine;
        for (let i = lastLine; i >= 0; i--) {
            const lineContent = editor.getLine(i).trim();
            if (!lineContent.startsWith("[") && (!lineContent.endsWith("]") || !lineContent.endsWith(")"))) {
                targetLine = i;
                break;
            }
        }
        const targetChar = editor.getLine(targetLine).length;
        editor.setCursor({line: targetLine, ch: targetChar});
        editor.scrollTo(null, 0);
        // TODO only focus if there's no quick dialogue
        editor.focus();
    }, 100);
}

async function saveFile() {
    const folder = editor.currentFolder;
    const filename = editor.currentFile;
    const fileData = zettel[folder][filename];
    if (fileData && fileData.handle) {
        let content = editor.getValue();
        const header = filename.replace('.md', '').replace(/^\w/, (c) => c.toUpperCase());
        content = content.trimStart();
        if (content.startsWith(`# ${header}`)) {
            content = content.slice(`# ${header}`.length).trimStart();
        }

        const writable = await fileData.handle.createWritable();
        await writable.write(content);
        await writable.close();
    } else {
        alert(`Cannot save ${filename}. No file handle found.`);
    }
}

function initDB() {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open('zettelApp', 1);
        request.onerror = () => reject(request.error);
        request.onsuccess = () => resolve(request.result);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains('handles')) {
                db.createObjectStore('handles');
            }
        };
    });
}

async function saveDirectoryHandle(directoryHandle) {
    const db = await initDB();
    const transaction = db.transaction('handles', 'readwrite');
    const store = transaction.objectStore('handles');
    await store.put(directoryHandle, 'savedDirectoryHandle');
}

async function getSavedDirectoryHandle() {
    const db = await initDB();
    return new Promise((resolve, reject) => {
        const transaction = db.transaction('handles', 'readonly');
        const store = transaction.objectStore('handles');
        const request = store.get('savedDirectoryHandle');
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

async function init(el) {
    initHyperMD(el);
    buildSidebar();
    const savedDirectoryHandle = await getSavedDirectoryHandle();
    if (savedDirectoryHandle instanceof FileSystemDirectoryHandle) {
        const permission = await savedDirectoryHandle.queryPermission({mode: 'read'});
        if (permission !== 'granted') {
            document.getElementById('welcome').style.display = 'flex';
        }
        zettel = {};
        await loadDirectory(savedDirectoryHandle);
        await showRandomFile();
    } else {
        document.getElementById('welcome').style.display = 'flex';
    }
}

function updateFocusedItem(resultsList) {
    document.querySelectorAll('#goToFileResults li').forEach(li => li.classList.remove('focused'));
    resultsList.forEach((item, index) => {
        if (index === focusedItemIndex) {
            item.classList.add('focused');
            item.scrollIntoView({block: "nearest"});
        } else {
            item.classList.remove('focused');
        }
    });
}

function openSearchModal() {
    document.getElementById('goToFile').style.display = 'block';
    const inputField = document.getElementById('goToFileInput');
    inputField.focus();

    searchList = Object.keys(zettel)
        .filter(folder => folder !== 'img')
        .flatMap(folder =>
            Object.keys(zettel[folder]).map(filename => ({folder, filename}))
        );
    focusedItemIndex = -1;
    const goToFileResults = document.getElementById('goToFileResults');
    goToFileResults.innerHTML = '';
}

document.addEventListener('keydown', (event) => {
    if (event.metaKey && event.key === 'p') {
        event.preventDefault();
        document.getElementById('goToFileInput').value = ''
        openSearchModal();
    }

    if (event.metaKey && event.key === 'k') {
        event.preventDefault();
        document.getElementById('goToFileInput').value = ''
        openSearchModal();
    }
});

function closeSearchModal() {
    document.getElementById('goToFile').style.display = 'none';
}

function similarity(s1, s2) {
    const distance = levenshtein(s1.toLowerCase(), s2.toLowerCase());
    const maxLength = Math.max(s1.length, s2.length);

    if (maxLength === 0) return 100;

    const similarity = (1 - distance / maxLength) * 100;

    return similarity.toFixed(2);
}

function levenshtein(s1, s2) {
    if (s1 === s2) {
        return 0;
    }
    var n = s1.length, m = s2.length;
    if (n === 0 || m === 0) {
        return n + m;
    }
    var x = 0, y, a, b, c, d, g, h, k;
    var p = new Array(n);
    for (y = 0; y < n;) {
        p[y] = ++y;
    }

    for (; (x + 3) < m; x += 4) {
        var e1 = s2.charCodeAt(x);
        var e2 = s2.charCodeAt(x + 1);
        var e3 = s2.charCodeAt(x + 2);
        var e4 = s2.charCodeAt(x + 3);
        c = x;
        b = x + 1;
        d = x + 2;
        g = x + 3;
        h = x + 4;
        for (y = 0; y < n; y++) {
            k = s1.charCodeAt(y);
            a = p[y];
            if (a < c || b < c) {
                c = (a > b ? b + 1 : a + 1);
            }
            else {
                if (e1 !== k) {
                    c++;
                }
            }

            if (c < b || d < b) {
                b = (c > d ? d + 1 : c + 1);
            }
            else {
                if (e2 !== k) {
                    b++;
                }
            }

            if (b < d || g < d) {
                d = (b > g ? g + 1 : b + 1);
            }
            else {
                if (e3 !== k) {
                    d++;
                }
            }

            if (d < g || h < g) {
                g = (d > h ? h + 1 : d + 1);
            }
            else {
                if (e4 !== k) {
                    g++;
                }
            }
            p[y] = h = g;
            g = d;
            d = b;
            b = c;
            c = a;
        }
    }

    for (; x < m;) {
        var e = s2.charCodeAt(x);
        c = x;
        d = ++x;
        for (y = 0; y < n; y++) {
            a = p[y];
            if (a < c || d < c) {
                d = (a > d ? d + 1 : a + 1);
            }
            else {
                if (e !== s1.charCodeAt(y)) {
                    d = c + 1;
                }
                else {
                    d = c;
                }
            }
            p[y] = d;
            c = a;
        }
        h = d;
    }

    return h;
}

function filterFiles() {
    const search = document.getElementById('goToFileInput').value.toLowerCase();
    const list = document.getElementById('goToFileResults');
    list.innerHTML = '';

    const lowPriorityFolders = ["archive", "_read_", "_watch_", "_shop_", "habits", "triggers", "today", "later"];

    let results = [];

    // Levenshtein distance
    searchList.forEach(({filename, folder}) => {
        const potentialMatch = filename.replace(/\.md$/, "");
        let similarityScore = similarity(search, potentialMatch);
        if (similarityScore >= 70) {
            if (lowPriorityFolders.includes(folder)) {
                similarityScore -= 30;
            }
            results.push({filename: filename, folder: folder, score: similarityScore});
        }
    });

    // Substring
    searchList.forEach(({filename, folder}) => {
        const potentialMatch = filename.replace(/\.md$/, "");
        const isSubstringMatch = potentialMatch.toLowerCase().includes(search.toLowerCase());
        if (!isSubstringMatch) {
            return;
        }

        let matchedPercent = search.length / potentialMatch.length * 100;
        results.push({
            filename: filename,
            folder: folder,
            score: Math.round(matchedPercent)
        });
    });

    const uniqueResultsMap = new Map();
    for (let i = 0; i < results.length; i++) {
        const item = results[i];
        const key = `${item.filename}-${item.folder}`;

        if (!uniqueResultsMap.has(key) || uniqueResultsMap.get(key).score < item.score) {
            uniqueResultsMap.set(key, item);
        }
    }
    results = Array.from(uniqueResultsMap.values()).sort((a, b) => b.score - a.score);

    results.forEach(({folder, filename}, index) => {
        const listItem = document.createElement('li');
        let title = filename.replace(/\.md$/, "")
        if (folder !== '') {
            listItem.textContent = `${folder}/${title}`;
        } else {
            listItem.textContent = title;
        }
        listItem.setAttribute('data-path', `${folder}/${filename}`);
        listItem.setAttribute('data-index', index);
        listItem.onclick = () => {
            showFile(folder, filename);
            closeSearchModal();
        };
        listItem.onmouseenter = () => {
            document.querySelectorAll('#goToFileResults li').forEach(li => li.classList.remove('focused'));
            listItem.classList.add('focused');
            focusedItemIndex = index;
        };
        list.appendChild(listItem);
    });


    focusedItemIndex = 0;
    updateFocusedItem(list.querySelectorAll('li'));
}

function closeGoToFile() {
    document.getElementById('goToFile').style.display = 'none';
}

document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
        closeGoToFile();
    }
});

// Toggle focus mode
document.addEventListener('keydown', function (event) {
    if ((event.altKey || event.metaKey) && event.key === 'Enter') {
        event.preventDefault();
        const sidebar = document.getElementById('editor-sidebar');
        console.log(sidebar.style.display);
        if (sidebar.style.display === 'none') {
            sidebar.style.display = 'block';
        } else {
            sidebar.style.display = 'none'; // Hide the sidebar
        }
    }
});

window.addEventListener('popstate', (event) => {
    // event.preventDefault();
    const state = event.state; // Get the state object
    if (state) {
        showFile(state['folder'], state['file'], false);
    }
});

document.getElementById('goToFile').addEventListener('keydown', (event) => {
    const resultsList = document.getElementById('goToFileResults').querySelectorAll('li');

    if (event.key === 'Enter') {
        event.preventDefault();
        if (resultsList[focusedItemIndex]) {
            const [folder, filename] = resultsList[focusedItemIndex].getAttribute('data-path').split('/');
            showFile(folder, filename);
            closeSearchModal();
        }
    }
});