#!/usr/bin/env python3

import os
import sys
from openai import OpenAI
import requests
import re

client = OpenAI(base_url = "http://127.0.0.1:8000/v1", api_key="akhfbsaeklg")

TIME_RANGE_PATTERN = re.compile(r"(\d{2}:\d{2}\.\d{3}) --> (\d{2}:\d{2}\.\d{3})")

def condense_vtt_transcript(filepath):
    """
    Handle two cases:
      1) Diarized VTTs (lines start with '[SPEAKER_x]:')
      2) Non-diarized VTTs (no speaker tags) -> remove times, remove any speaker labels, 
         and output text as single lines (one line per time block).
    """
    with open(filepath, 'r') as file:
        transcript = file.read().splitlines()

    # Check if at least one line starts with '[' => diarized
    diarized = any(line.startswith('[') for line in transcript)

    if diarized:
        # ---------- Diarized logic (unchanged) ----------
        condensed_transcript = []
        current_speaker = None
        current_speech = ""
        start_time = None
        end_time = None

        for i, line in enumerate(transcript):
            # If you see a line with "[SPEAKER_x]: some text"
            if line.startswith('[') and ']: ' in line:
                speaker, speech = line.split(']: ', 1)
                speaker = speaker + ']'
                # The line before should be the time range "00:00.651 --> 00:28.203"
                # Make sure we have a valid time range line
                if i > 0 and TIME_RANGE_PATTERN.search(transcript[i - 1]):
                    range_match = TIME_RANGE_PATTERN.search(transcript[i - 1])
                    time_start, time_end = range_match.groups()
                else:
                    # If there's an unexpected format, just skip
                    continue

                if current_speaker is None:
                    # First speaker block
                    current_speaker = speaker
                    current_speech = speech.strip()
                    start_time, end_time = time_start, time_end
                elif speaker == current_speaker:
                    # Same speaker, accumulate text
                    current_speech += " " + speech.strip()
                    end_time = time_end
                else:
                    # New speaker => close out old block
                    condensed_transcript.append(
                        f'{start_time} --> {end_time}\n{current_speaker}: {current_speech}'
                    )
                    current_speaker = speaker
                    current_speech = speech.strip()
                    start_time, end_time = time_start, time_end

        # Handle the very last speaker block
        if current_speaker is not None:
            condensed_transcript.append(
                f'{start_time} --> {end_time}\n{current_speaker}: {current_speech}'
            )
        return condensed_transcript

    else:
        # ---------- Non-diarized logic (NEW) ----------
        #
        # We remove all timestamps and simply collect each block of text between
        # timestamps into a single line. Also omit any "WEBVTT" or blank lines.
        #
        condensed_transcript = []
        current_block = []

        for line in transcript:
            stripped_line = line.strip()
            if not stripped_line or stripped_line.upper() == "WEBVTT":
                # Skip empty lines or "WEBVTT" header
                continue

            # If this is a time-range line, we treat it as a boundary:
            if TIME_RANGE_PATTERN.search(stripped_line):
                # If there's any accumulated text, push it as one line
                if current_block:
                    condensed_transcript.append(' '.join(current_block))
                    current_block = []
            else:
                # It's a content line, just accumulate it
                current_block.append(stripped_line)

        # If anything remains at the end, append it
        if current_block:
            condensed_transcript.append(' '.join(current_block))

        return condensed_transcript

# TODO XXX:
# expose tokenize() llamacpp function as API and use here
# for now - approximate
def stub_token_count(text):
    """
    Stub function to compute approximate tokens.
    For now, just guess ~1 token per word as a placeholder.
    """
    return len(text.split())

def chunk_transcript(transcript, total_tokens, max_tokens_per_chunk, big_speech_len):
    chunks = []
    if total_tokens <= max_tokens_per_chunk:
        chunks.append(transcript)
        return chunks

    # Attempt naive chunking by line
    chunks_num = total_tokens // max_tokens_per_chunk
    if total_tokens % max_tokens_per_chunk > 0:
        chunks_num += 1

    lines = transcript.split('\n')

    # Calculate the initial position of line separators, dividing the text evenly
    # Note: first separator is always at index 0
    # NOTE: the following divides according to lines, not tokens
    # If there is just one speaker, this will not work well, but hopefully single-speaker videos are
    # small enough to fit in max_tokens_per_chunk as one chunk in the first condition in this function
    line_separators = [i * (len(lines) // chunks_num) for i in range(chunks_num)]

    # Adjust the line separators in reverse
    for i in range(len(line_separators) - 1, 0, -1):
        sep = line_separators[i]
        # Attempt to ensure large lines remain in one chunk
        while sep < len(lines) - 1 and len(lines[sep].split(':', 1)[-1]) < big_speech_len:
            sep += 1	# Move forward to find a big speech
            if i < len(line_separators) - 1 and sep >= line_separators[i + 1]:	# Ensure not to cross the next separator
                sep = line_separators[i + 1] - 1	# Adjust to not cross over
                break
        line_separators[i] = sep
    # No need to adjust the first separator (i=0), it should always remain as 0

    # Split transcript into chunks
    for i in range(len(line_separators) - 1):
        chunk = '\n'.join(lines[line_separators[i]:line_separators[i + 1]])
        chunks.append(chunk)

    # Add the last chunk
    chunks.append('\n'.join(lines[line_separators[-1]:]))

    print(f"Total number of lines in transcript: {len(lines)}, chunks_num={len(chunks)}", file=sys.stderr)
    print(f"Line separators: {line_separators}", file=sys.stderr)
    return chunks

def print_text_var(textvar, label):
    #DEBUG FUNC
    #print(f"\nPRINTING {label} START:\n{textvar}\nPRINTING {label} END.\n")
    pass

def run_summarization_pipeline(transcript_file_path):
    # 1) Condense transcript
    condensed_lines = condense_vtt_transcript(transcript_file_path)

    # For chunking & summarization steps, just join with newlines
    condensed = '\n'.join(condensed_lines)
    print_text_var(condensed, "condensed")

    # 2) Stubbed token length
    transcript_token_length = stub_token_count(condensed)
    print("Approximate token length:", transcript_token_length, file=sys.stderr)

    # 3) Chunk transcript
    max_tokens_per_chunk = 22000
    big_speech_len = 100
    chunks = chunk_transcript(condensed, transcript_token_length, max_tokens_per_chunk, big_speech_len)
    print_text_var(str(len(chunks)), "len of chunks")

    # 4) Summarize each chunk with OpenAI-compatible llama-cpp server
    preprompt = (
        "Write a detailed summary of the following transcript from a work meeting. "
        "Organize the content into clear, chronological paragraphs that maintain a natural narrative flow. "
        "Make sure to include all important details, technical insights, and notable terms, "
        "suitable for a technical reader. Ensure to integrate the contributions of all speakers, "
        "omitting only minor interjections. The summary should provide a comprehensive and detailed "
        "overview that logically progresses through the discussions, targeted at two pages in length.\n\n"
    )

    final_summary = ""

    for i, chunk_text in enumerate(chunks, start=1):
        print_text_var(chunk_text, f"CHUNK {i}")
        messages = [
            {"role": "user", "content": preprompt + chunk_text}
        ]
        try:
            response = client.chat.completions.create(
                model="any-model-name-here",  # Llama-cpp doesn't strictly use this, but let's put something
                messages=messages,
                temperature=0.7,
                max_tokens=8192,
            )
            chunk_summary = response.choices[0].message.content
            final_summary += chunk_summary + "\n\n"
        except Exception as e:
            print("Error calling openai.ChatCompletion:", str(e), file=sys.stderr)
            sys.exit(1)

    print_text_var(final_summary, "FINAL SUMMARY")
    return final_summary

def main():
    if len(sys.argv) != 2:
        print("Error: Invalid number of arguments.", file=sys.stderr)
        print("Usage: python generate_ai_summary.py <transcript.vtt>", file=sys.stderr)
        sys.exit(1)

    vtt_transcript_file_path = sys.argv[1]
    if not vtt_transcript_file_path.endswith('.vtt'):
        print("Error: The provided file is not a .vtt file.", file=sys.stderr)
        sys.exit(1)

    if not os.path.exists(vtt_transcript_file_path):
        print(f"Error: The file {vtt_transcript_file_path} does not exist.", file=sys.stderr)
        sys.exit(1)

    summary = run_summarization_pipeline(vtt_transcript_file_path)
    print(summary)

if __name__ == "__main__":
    main()

