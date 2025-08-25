#!/bin/bash

# dzdl - Deezer Download Helper Script
# A simplified wrapper for deezer-cli and godeez

set -euo pipefail

# Script configuration
SCRIPT_NAME="dzdl"
VERSION="1.0.0"
COMMAND_TYPE=""

# Default values
QUALITY="flac"
TYPE="track"
LIMIT=1
SKIP_CONFIRMATION=false
BPM_FLAG=""
SEARCH_IDS=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Print colored output
print_color() {
    local color=$1
    shift
    printf "${color}%s${NC}\n" "$*"
}

# Print usage information
show_help() {
    printf "${BOLD}${SCRIPT_NAME}${NC} v${VERSION} - Simplified Deezer Download Helper\n\n"
    printf "${BOLD}USAGE:${NC}\n"
    printf "    %s <command> [OPTIONS] <search_query>\n" "$SCRIPT_NAME"
    printf "    %s [OPTIONS] <search_query>  ${BLUE}# defaults to track${NC}\n\n" "$SCRIPT_NAME"
    printf "${BOLD}COMMANDS:${NC}\n"
    printf "    track, t               Search and download tracks (default)\n"
    printf "    album, a               Search and download albums\n"
    printf "    artist, ar             Search and download artist discography\n"
    printf "    playlist, p            Search and download playlists\n"
    printf "    quick, q               Quick track download (no confirmation)\n\n"
    printf "${BOLD}OPTIONS:${NC}\n"
    printf "    -q, --quality QUALITY   Download quality: flac, 320, 128\n"
    printf "                           (default: flac)\n"
    printf "    -l, --limit NUMBER      Number of search results to fetch\n"
    printf "                           (default: 1)\n"
    printf "    -y, --yes              Skip confirmation prompts\n"
    printf "    -b, --bpm              Include BPM information in download\n"
    printf "    -h, --help             Show this help message\n"
    printf "    -v, --version          Show version information\n\n"
    printf "${BOLD}EXAMPLES:${NC}\n"
    printf "    %s \"pink floyd\"                    # search tracks\n" "$SCRIPT_NAME"
    printf "    %s track \"another brick\"           # same as above\n" "$SCRIPT_NAME"
    printf "    %s album \"dark side of the moon\"   # search albums\n" "$SCRIPT_NAME"
    printf "    %s a -l 5 \"pink floyd\"            # 5 albums by Pink Floyd\n" "$SCRIPT_NAME"
    printf "    %s playlist \"rock classics\"        # search playlists\n" "$SCRIPT_NAME"
    printf "    %s p -q 320 --yes \"80s hits\"     # auto-download playlist\n" "$SCRIPT_NAME"
    printf "    %s quick \"money pink floyd\"        # quick download, no confirmation\n" "$SCRIPT_NAME"
    printf "    %s q --bpm \"bohemian rhapsody\"    # quick with BPM\n" "$SCRIPT_NAME"
    printf "    %s artist \"pink floyd\"             # artist discography\n\n" "$SCRIPT_NAME"
    printf "${BOLD}REQUIREMENTS:${NC}\n"
    printf "    - deezer-cli: For searching Deezer catalog\n"
    printf "    - godeez: For downloading tracks\n\n"
    printf "${BOLD}AUTHOR:${NC}\n"
    printf "    Created for simplified Deezer downloading workflow\n"
}

# Show version
show_version() {
    echo "$SCRIPT_NAME v$VERSION"
}

# Check if required commands exist
check_dependencies() {
    local missing_deps=()
    
    if ! command -v deezer-cli &> /dev/null; then
        missing_deps+=("deezer-cli")
    fi
    
    if ! command -v godeez &> /dev/null; then
        missing_deps+=("godeez")
    fi
    
    if [ ${#missing_deps[@]} -gt 0 ]; then
        print_color "$RED" "Error: Missing required dependencies:"
        for dep in "${missing_deps[@]}"; do
            print_color "$RED" "  - $dep"
        done
        print_color "$YELLOW" "Please install the missing dependencies and try again."
        exit 1
    fi
}

# Parse command type
parse_command() {
    case "$1" in
        track|t)
            TYPE="track"
            COMMAND_TYPE="track"
            return 0
            ;;
        album|a)
            TYPE="album"
            COMMAND_TYPE="album"
            return 0
            ;;
        artist|ar)
            TYPE="artist"
            COMMAND_TYPE="artist"
            return 0
            ;;
        playlist|p)
            TYPE="playlist"
            COMMAND_TYPE="playlist"
            return 0
            ;;
        quick|q)
            TYPE="track"
            COMMAND_TYPE="quick"
            SKIP_CONFIRMATION=true
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Validate search type
validate_type() {
    case "$1" in
        track|album|artist|playlist)
            return 0
            ;;
        *)
            print_color "$RED" "Error: Invalid type '$1'. Must be one of: track, album, artist, playlist"
            exit 1
            ;;
    esac
}

# Validate quality
validate_quality() {
    case "$1" in
        flac|320|128)
            return 0
            ;;
        *)
            print_color "$RED" "Error: Invalid quality '$1'. Must be one of: flac, 320, 128"
            exit 1
            ;;
    esac
}

# Validate limit
validate_limit() {
    if ! [[ "$1" =~ ^[1-9][0-9]*$ ]]; then
        print_color "$RED" "Error: Limit must be a positive integer"
        exit 1
    fi
    
    if [ "$1" -gt 50 ]; then
        print_color "$YELLOW" "Warning: Large limit ($1) may result in many results"
        read -p "Continue? [y/N]: " -r
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_color "$BLUE" "Operation cancelled."
            exit 0
        fi
    fi
}

# Search for content
search_content() {
    local query="$1"
    
    print_color "$BLUE" "Searching for: $query"
    print_color "$BLUE" "Type: $TYPE, Limit: $LIMIT"
    echo
    
    # First, check if we get any results
    local test_results
    if ! test_results=$(deezer-cli search "$query" --type "$TYPE" -l 1 -o ids 2>/dev/null); then
        print_color "$RED" "Error: Search failed. Please check your query and try again."
        exit 1
    fi
    
    # Check if results are empty
    if [ -z "$test_results" ] || [ "$test_results" = "" ]; then
        print_color "$RED" "No results found for: $query"
        exit 1
    fi
    
    # Now get the actual results for display
    local search_results
    if ! search_results=$(deezer-cli search "$query" --type "$TYPE" -l "$LIMIT" -o table 2>/dev/null); then
        print_color "$RED" "Error: Failed to fetch search results for display"
        exit 1
    fi
    
    print_color "$GREEN" "Search Results:"
    echo "$search_results"
    echo
    
    # Get IDs for downloading
    local ids
    if ! ids=$(deezer-cli search "$query" --type "$TYPE" -l "$LIMIT" -o ids 2>/dev/null); then
        print_color "$RED" "Error: Failed to fetch IDs"
        exit 1
    fi
    
    # Verify we got the expected number of results
    local actual_count
    actual_count=$(echo "$ids" | grep -c '^[0-9]\+' || echo "0")
    
    if [ "$actual_count" -eq 0 ]; then
        print_color "$RED" "No valid IDs found"
        exit 1
    fi
    
    if [ "$actual_count" -ne "$LIMIT" ] && [ "$LIMIT" -gt 1 ]; then
        print_color "$YELLOW" "Note: Found $actual_count results instead of requested $LIMIT"
    fi
    
    # Store IDs in global variable instead of echoing
    SEARCH_IDS="$ids"
}

# Confirm download
confirm_download() {
    local ids="$1"
    local count
    count=$(echo "$ids" | wc -l)
    
    if [ "$SKIP_CONFIRMATION" = true ]; then
        return 0
    fi
    
    echo
    print_color "$YELLOW" "Ready to download $count item(s) in $QUALITY quality"
    if [ "$BPM_FLAG" = "--bpm" ]; then
        print_color "$YELLOW" "BPM information will be included"
    fi
    
    read -p "Proceed with download? [Y/n]: " -r
    if [[ $REPLY =~ ^[Nn]$ ]]; then
        print_color "$BLUE" "Download cancelled."
        exit 0
    fi
}

# Download content
download_content() {
    local ids="$1"
    local total_count
    local current=0
    
    total_count=$(echo "$ids" | wc -l)
    
    print_color "$GREEN" "Starting download..."
    echo
    
    # Download each ID separately
    while IFS= read -r id; do
        if [ -n "$id" ] && [[ "$id" =~ ^[0-9]+$ ]]; then
            current=$((current + 1))
            print_color "$BLUE" "Downloading [$current/$total_count]: ID $id"
            
            # Build godeez command based on the search type
            local cmd="godeez download $TYPE $id --quality $QUALITY"
            if [ "$BPM_FLAG" = "--bpm" ]; then
                cmd="$cmd --bpm"
            fi
            
            # Execute download
            if eval "$cmd"; then
                print_color "$GREEN" "✓ Successfully downloaded ID $id"
            else
                print_color "$RED" "✗ Failed to download ID $id"
            fi
            echo
        fi
    done <<< "$ids"
    
    print_color "$GREEN" "Download process completed!"
}

# Main function
main() {
    local query=""
    
    # Check if first argument is a command
    if [[ $# -gt 0 ]] && parse_command "$1"; then
        shift # Remove the command from arguments
    fi
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -t|--type)
                TYPE="$2"
                validate_type "$TYPE"
                shift 2
                ;;
            -q|--quality)
                QUALITY="$2"
                validate_quality "$QUALITY"
                shift 2
                ;;
            -l|--limit)
                LIMIT="$2"
                validate_limit "$LIMIT"
                shift 2
                ;;
            -y|--yes)
                SKIP_CONFIRMATION=true
                shift
                ;;
            -b|--bpm)
                BPM_FLAG="--bpm"
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            -v|--version)
                show_version
                exit 0
                ;;
            -*)
                print_color "$RED" "Error: Unknown option $1"
                echo "Use -h or --help for usage information."
                exit 1
                ;;
            *)
                if [ -z "$query" ]; then
                    query="$1"
                else
                    query="$query $1"
                fi
                shift
                ;;
        esac
    done
    
    # Check if query is provided
    if [ -z "$query" ]; then
        print_color "$RED" "Error: Search query is required"
        echo "Use -h or --help for usage information."
        exit 1
    fi
    
    # Check dependencies
    check_dependencies
    
    # Search for content
    search_content "$query"
    
    # Confirm download
    confirm_download "$SEARCH_IDS"
    
    # Download content
    download_content "$SEARCH_IDS"
}

# Handle interruption
trap 'print_color "$YELLOW" "\nOperation interrupted."; exit 130' INT

# Run main function
main "$@"
