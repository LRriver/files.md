// Files structure:
// {
//   "dir": [
//     {
//       "filename": [
//         {
//           content: "File content here...",
//           lastModified: <timestamp>,
//           handle: <file handle>,
//           imageUrl: <image url if any>
//         },
//         ...
//       ]
//     },
//     ...
//   ]
// }
let files = [];
const supportedFileTypes = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const systemDirs = ["img", "archive", "_read_", "_watch_", "_shop_", "today", "later", "journal", "habits", "triggers", "places"];

let filesMetadata = {files: {}, timestamps: {}};
const SYNC_STORAGE_KEY = 'files';

// Returns files in flattened structure:
// {
//   "dir": {
//      ...
//   },
//   "dir/dir2": {
//      ...
//   },
// }
// The code is quite messy. We have to make lots of optimizations,
// otherwise it's going to be slow even with 5K files.
async function loadLocalFiles(dirHandle) {
    let newFiles = {};

    async function loadDir(dirHandle, path = "", depth = 1) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (const entry of entries) {
            const filename = entry.name.normalize("NFC");

            if (entry.kind === 'directory') {
                if (filename.startsWith('.') || depth >= 5) continue;

                const dir = `${path}${filename}/`;
                newFiles[filename] = {};
                dirPromises.push({handle: entry, dir, depth: depth + 1});
            } else if (entry.kind === 'file' && supportedFileTypes.includes(filename.split('.').pop())) {
                const dir = path.replace(/\/+$/, '');
                if (!newFiles[dir]) newFiles[dir] = {};

                // Reuse existing file handle if it exists
                if (files?.[dir]?.[filename] !== undefined) {
                    newFiles[dir][filename] = files[dir][filename];
                    continue;
                }
                newFiles[dir][filename] = {handle: entry};

                entry.getFile().then(file => {
                    newFiles[dir][filename].lastModified = file.lastModified;
                });

                if (dir === 'img') {
                    getImageUrl(entry).then(imageUrl => {
                        newFiles[dir][filename].imageUrl = imageUrl;
                    });
                }
            }
        }

        await Promise.all(dirPromises.map(({handle, dir, depth}) =>
            loadDir(handle, dir, depth)
        ));
    }

    await loadDir(dirHandle);

    // Remove empty dirs
    for (const dir in newFiles) {
        if (Object.keys(newFiles[dir]).length === 0) {
            delete newFiles[dir];
        }
    }

    // Load metadata
    const savedStates = localStorage.getItem(SYNC_STORAGE_KEY);
    if (savedStates) {
        filesMetadata = JSON.parse(savedStates);
    }

    return newFiles;
}

async function syncWithServer() {
    console.log("Starting sync with server...");

    // Send locally modified files and timestamps of last seen dirs from server
    let filesToSend = [];
    filesToSend = await collectModifiedFiles();
    let response = await fetch('https://habits.files.md/sync', {
        method: 'POST',
        headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
        body: JSON.stringify({
            files: filesToSend, // files that I have
            timestamps: filesMetadata['timestamps'] || [], // last seen server dir timestamps
        })
    });
    if (!response.ok) {
        console.log(`Server responded with ${response.status}`);
        return
    }

    const server = await response.json();
    for (const fileInfo of server.files) {
        const {path, content, lastModified} = fileInfo;

        // What about more than 2 levels nested?
        let dir, filename;
        if (path.includes('/')) {
            const parts = path.split('/');
            filename = parts.pop();
            dir = parts.join('/');
        } else {
            dir = '';
            filename = path;
        }

        // TODO if file was modified locally, we need to re-read it before writing.
        const dirs = path.split('/');
        dirs.pop() // remove filename
        let currentDirHandle = await getSavedDirectoryHandle();
        for (const dirName of dirs) {
            if (dirName) {
                currentDirHandle = await currentDirHandle.getDirectoryHandle(dirName, {create: true});
            }
        }

        // TODO create dirs if not exist
        console.log("Syncing " + filename);
        let fileHandle;
        try {
            fileHandle = await currentDirHandle.getFileHandle(filename, {create: true});
        } catch (error) {
            console.error(`Error getting file handle for '${dir}/${filename}':`, error);
            continue;
        }
        let file = await fileHandle.getFile()
        let clientHash = hash(await file.text());
        let serverHash = hash(content);
        if (clientHash !== serverHash) {
            console.log("Hashes do not match, writing file...");
            const writable = await fileHandle.createWritable();
            await writable.write(content);
            await writable.close();
        } else {
            console.log("Hashes match, no need to write file.");
        }
        if (!filesMetadata['files'][dir]) filesMetadata['files'][dir] = {};
        filesMetadata['files'][dir][filename] = {
            hash: hash(content),
            lastModified: lastModified,
            path: path
        };
    }
    filesMetadata['timestamps'] = server.timestamps;
    localStorage.setItem(SYNC_STORAGE_KEY, JSON.stringify(filesMetadata));
    console.log("Sync completed successfully");

}

async function collectModifiedFiles() {
    const filesToSend = [];
    const fileProcessingPromises = [];

    for (const dir in files) {
        if (dir === 'img') continue; // Skip image directory

        for (const filename in files[dir]) {
            const processPromise = collectFile(dir, filename)
                .then(result => {
                    if (result) filesToSend.push(result);
                });

            fileProcessingPromises.push(processPromise);
        }
    }

    await Promise.all(fileProcessingPromises);
    return filesToSend;
}

async function collectFile(dir, filename) {
    try {
        const fileData = files[dir][filename];
        if (!fileData?.handle) return null;

        const file = await fileData.handle.getFile();
        const content = await file.text();

        const path = filesMetadata?.files?.[dir]?.[filename]?.path;
        if (!path) {
            console.log(`File ${dir}/${filename} not found on server, skipping...`);
            return null;
        }

        const serverHash = filesMetadata?.files?.[dir]?.[filename]?.hash;
        const serverTime = filesMetadata?.files?.[dir]?.[filename]?.lastModified;

        if (serverHash !== hash(content)) {
            return {
                content,
                path,
                lastModified: serverTime,
            };
        }

        return null;
    } catch (error) {
        console.error(`Error processing ${dir}/${filename}:`, error);
        return null;
    }
}

async function saveFile() {
    const dir = editor.currentDir;
    const filename = editor.currentFile;
    const fileData = files[dir][filename];
    if (fileData && fileData.handle) {
        let content = getCurrentContent();
        const writable = await fileData.handle.createWritable();
        await writable.write(content);
        await writable.close(); // Buffer is flushed on disk at this moment, it could be interrupted by the event pool, so maintain a flag
    } else {
        if (fileData.handle) {
            alert(`Cannot save ${filename}. No file handle found.`);
        }
    }
}

function hash(str) {
    let hash = 0;
    for (let i = 0, len = str.length; i < len; i++) {
        let chr = str.charCodeAt(i);
        hash = (hash << 5) - hash + chr;
        hash |= 0;
    }
    return hash;
}