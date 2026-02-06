#!/bin/bash

# Output file
OUTPUT="full_context.md"

# Clear previous run
echo "# Project Dump $(date)" > "$OUTPUT"

# Define what to ignore
# Add any other folders you want to skip here
IGNORE_DIRS="-not -path '*/.git/*' -not -path '*/vendor/*' -not -path '*/bin/*' -not -path '*/.codepicker/*'"

echo "👀 Scanning directory... dumping to $OUTPUT"

# Find files
# We focus on .go, .mod, .sum, .json, .sql, .md, etc.
# You can tweak the extensions below.
find . -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" -o -name "*.json" \) \
    $IGNORE_DIRS \
    | sort | while read -r file; do

    # Skip the output file itself and this script
    if [[ "$file" == "./$OUTPUT" || "$file" == "./$0" ]]; then continue; fi

    # Get relative path for the header
    clean_path=${file#./}
    
    # Get extension for syntax highlighting
    ext="${file##*.}"
    
    echo "Processing: $clean_path"

    # Append to the markdown file
    {
        echo "## File: $clean_path"
        echo '```'$ext
        cat "$file"
        echo '```'
        echo ""
        echo "---"
        echo ""
    } >> "$OUTPUT"

done

echo "✅ Done! Upload '$OUTPUT' to the chat."
