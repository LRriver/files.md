// Various text functions, ported from Golang bot.

// Add content at the beginning of the file, prepending current's day header
async function addToFile(path, content) {
    let existingContent = '';

    existingContent = read(path);

    const now = new Date();
    const monthNames = [
        'January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'
    ];
    const dayNames = [
        'Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'
    ];

    const header = `#### ${now.getDate()} ${monthNames[now.getMonth()]} ${now.getFullYear()}, ${dayNames[now.getDay()]}`;
    const newContent = insertTextAfterHeader(existingContent, header, content);

    await write(path, newContent);
}

function insertTextAfterHeader(existingContent, header, newContent) {
    if (!existingContent.includes(header)) {
        if (existingContent === "") {
            return `${header}\n${newContent}`;
        } else {
            return `${header}\n${newContent}\n\n${existingContent}`;
        }
    }

    const lines = existingContent.split("\n");
    let headerIndex = -1;

    // Find the header line
    for (let i = 0; i < lines.length; i++) {
        if (lines[i] === header) {
            headerIndex = i;
            break;
        }
    }

    if (headerIndex === -1) {
        return `${header}\n${newContent}\n\n${existingContent}`;
    }

    // Find where to insert (after the last line belonging to this header)
    let insertIndex = headerIndex + 1;

    // Look for the next header or end of content
    for (let i = headerIndex + 1; i < lines.length; i++) {
        if (lines[i].startsWith("###")) {
            insertIndex = i;
            break;
        }
        // If we encounter an empty line, insert before it
        if (lines[i].trim() === "") {
            insertIndex = i;
            break;
        }
        insertIndex = i + 1;
    }

    // Insert the new content
    const newLines = [];
    newLines.push(...lines.slice(0, insertIndex));
    newLines.push(newContent);

    // Add empty line after new content if there's content following and it's not empty
    if (insertIndex < lines.length && lines[insertIndex].trim() !== "") {
        newLines.push("");
    }

    newLines.push(...lines.slice(insertIndex));

    return newLines.join("\n");
}

