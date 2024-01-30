import requests
import sys
import os

def condense_vtt_transcript(filepath):
    with open(filepath, 'r') as file:
        transcript = file.read().splitlines()

    condensed_transcript = []
    current_speaker = None
    current_speech = ""
    start_time = None
    end_time = None

    for i, line in enumerate(transcript):
        if line.startswith('['):
            speaker, speech = line.split(']: ', 1)
            speaker = speaker + ']'
            if current_speaker is None:
                current_speaker = speaker
                current_speech = speech.strip()
                start_time, end_time = transcript[i-1].split(' --> ')
            elif speaker == current_speaker:
                current_speech += " " + speech.strip()
                end_time = transcript[i-1].split(' --> ')[1]
            else:
                condensed_transcript.append(f'{start_time} --> {end_time}\n{current_speaker}: {current_speech}')
                current_speaker = speaker
                current_speech = speech.strip()
                start_time, end_time = transcript[i-1].split(' --> ')

    # for the last speaker
    condensed_transcript.append(f'{start_time} --> {end_time}\n{current_speaker}: {current_speech}')

    return condensed_transcript

def chunk_transcript(transcript, total_tokens, max_tokens_per_chunk, big_speech_len):
    chunks = []
    # If total tokens are less than max_tokens_per_chunk, just make it one chunk
    if total_tokens <= max_tokens_per_chunk:
        chunks.append(transcript)
        return chunks

    # Calculate initial number of chunks and adjust it
    chunks_num = total_tokens // max_tokens_per_chunk
    if total_tokens % max_tokens_per_chunk > 0:
        chunks_num += 1

    # Split the transcript into lines
    lines = transcript.split('\n')

    # Calculate the initial position of line separators, dividing the text evenly
    # Note: first separator is always at index 0
    # NOTE: the following divides according to lines, not tokens
    # If there is just one speaker, this will not work well, but hopefully single-speaker videos are
    # small enough to fit in max_tokens_per_chunk as one chunk in the first condition in this function
    line_separators = [i * (len(lines) // chunks_num) for i in range(chunks_num)]

    # Adjust the line separators in reverse
    for i in range(len(line_separators) - 1, 0, -1):  # Start from the last to the second item
        sep = line_separators[i]
        while sep < len(lines) - 1 and len(lines[sep].split(':', 1)[-1]) < big_speech_len:
            sep += 1  # Move forward to find a big speech
            if i < len(line_separators) - 1 and sep >= line_separators[i + 1]:  # Ensure not to cross the next separator
                sep = line_separators[i + 1] - 1  # Adjust to not cross over
                break
        line_separators[i] = sep
    # No need to adjust the first separator (i=0), it should always remain as 0

    # Split the transcript into chunks
    for i in range(len(line_separators) - 1):
        chunk = '\n'.join(lines[line_separators[i]:line_separators[i + 1]])
        chunks.append(chunk)

    # Add the last chunk
    chunks.append('\n'.join(lines[line_separators[-1]:]))

    # Print the total number of lines in the transcript
    print(f"Total number of lines in transcript: {len(lines)}, chunks_num={len(chunks)}", file=sys.stderr)

    # Print the whole line_separators list
    print(f"Line separators: {line_separators}", file=sys.stderr)

    return chunks

def get_token_length_from_koboldcpp(text_to_tokenize):
    # TODO: get not just total amount of tokens, but also tokens per line
    # for more accurate chunking 
    tokencount_api_endpoint = "http://localhost:5001/api/extra/tokencount"
    payload = {
        "prompt": text_to_tokenize
    }
    response = requests.post(tokencount_api_endpoint, json=payload)
    # Check if the request was successful
    if response.status_code == 200:
        if 'value' in response.json():
            transcript_token_length = int(response.json()['value'])
            print('Transcript token length is ' + str(transcript_token_length), file=sys.stderr)
            return transcript_token_length
    else:
        print("Request failed.", file=sys.stderr)
        print("Status Code:" + response.status_code, file=sys.stderr)
        print("Response Text:" + response.text, file=sys.stderr)
        exit(1)

# Function to print the usage information
def print_usage():
    print("Usage: python run_summarization_pipeline.py <transcript.vtt>", file=sys.stderr)
    print("       <transcript.vtt> is the path to a .vtt file containing the transcript.", file=sys.stderr)

#############################################################################

def run_summarization_pipeline(transcript_file_path):
    # 1) First condense transcript to save some tokens:
    condensed_lines = condense_vtt_transcript(transcript_file_path)
    condensed = '\n'.join(condensed_lines) 

    #with open('condensed_transcript.txt', 'w') as output_file:
    #    output_file.write(condensed)
        #print('Condensed transcript written to condensed_transcript.txt')

    # 2) Check token length of condensed transcript:
    transcript_token_length = get_token_length_from_koboldcpp(condensed)

    # 3) Chunk transcript according to token length:

    max_tokens_per_chunk = 22000 # Around as much context as one can fit on a 24G VRAM GPU for Mixtral
    big_speech_len = 100 # Align chunk start to big speeches for better logical separation

    chunks = chunk_transcript(condensed, transcript_token_length, max_tokens_per_chunk, big_speech_len)

    # Iterate over the chunks and write each one to a separate file
#    for i, chunk in enumerate(chunks, start=1):
#        chunk_filename = f'condensed_chunk{i}.txt'
#        with open(chunk_filename, 'w') as chunk_file:
#            chunk_file.write(chunk)
            #print(f'Chunk {i} written to {chunk_filename}')


    # 4) Ask LLM to summarize each chunk separately, then concatenate outputs

    # Set the API endpoint URL
    generate_api_endpoint =   "http://localhost:5001/api/v1/generate"

    # Create the JSON payload
    payload = {
          "prompt": "",
          "max_context_length": 32768,
          "max_length": 2048,
          "rep_pen": 1.1,
          "temperature": 0.7,
          "top_p": 0.92,
          "min_p": 0,
          "top_k": 100
    }

#    preprompt = "Summarize the following transcript from a work meeting in chronological sections. Make sure to include all relevant details, be concrete and avoid low-information statements. Include technical information and keywords as well, the summary is for a technical person. Quote the stories and statements that were impactful and stood out. Make sure that the speech of every speaker is included unless they were just briefly interjecting. The summary should be long enough to include all important details and overall should fill roughly two pages.\n\n"
    preprompt = "Write a detailed summary of the following transcript from a work meeting. Organize the content into clear, chronological paragraphs that maintain a natural narrative flow. Make sure to include all important details, technical insights and notable terms, suitable for a technical reader. Ensure to integrate the contributions of all speakers, omitting only minor interjections. The summary should provide a comprehensive and detailed overview that logically progresses through the discussions, targeted at two pages in length.\n\n"
#    preprompt = "Summarize the following transcript: \n\n"

    output_summary = ""

    for chunk in chunks:
        payload['prompt'] = preprompt + chunk
        payload['prompt'] = "[INST]" + payload['prompt'] + "[/INST]\n"
        # Make the API request
        response = requests.post(generate_api_endpoint, json=payload)

        # Check if the request was successful
        if response.status_code == 200:
            #print("Request was successful.")
            if 'results' in response.json():
                output_summary += response.json()['results'][0]['text']
                output_summary += "\n\n"
        else:
            print("Request failed.", file=sys.stderr)
            print("Status Code:" + response.status_code, file=sys.stderr)
            print("Response Text:" + response.text, file=sys.stderr)
            sys.exit(1)


#    with open('output_summary.txt', 'w') as output_file:
#        output_file.write(output_summary)
        #print('Summary written to output_summary.txt')

    return output_summary


if __name__ == "__main__":
    # Check if the number of command-line arguments is correct
    if len(sys.argv) != 2:
        print("Error: Invalid number of arguments.", file=sys.stderr)
        print_usage()
        sys.exit(1)  # Exit with a non-zero status to indicate an error

    vtt_transcript_file_path = sys.argv[1]
    # Check if the provided file path has a .vtt extension
    if not vtt_transcript_file_path.endswith('.vtt'):
        print("Error: The provided file is not a .vtt file.", file=sys.stderr)
        print_usage()
        sys.exit(1)

    # Check if the file exists
    if not os.path.exists(vtt_transcript_file_path):
        print(f"Error: The file {vtt_transcript_file_path} does not exist.", file=sys.stderr)
        sys.exit(1)

    summary = run_summarization_pipeline(vtt_transcript_file_path)
    # Send summary to stdout
    print(summary)
